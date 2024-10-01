// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package trial

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"time"

	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	licensing "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	name              = "trial-controller"
	EULAValidationMsg = `Please set the annotation elastic.co/eula to "accepted" to accept the EULA`
	trialOnlyOnceMsg  = "trial can be started only once"
)

var (
	userFriendlyMsgs = map[licensing.LicenseStatus]string{
		licensing.LicenseStatusInvalid: "trial license signature invalid",
		licensing.LicenseStatusExpired: "trial license expired",
	}
)

// ReconcileTrials reconciles Enterprise trial licenses.
type ReconcileTrials struct {
	k8s.Client
	operator.Parameters
	recorder record.EventRecorder
	// iteration is the number of times this controller has run its Reconcile method.
	iteration  uint64
	trialState licensing.TrialState
}

// Reconcile watches a trial status secret. If it finds a trial license it checks whether a trial has been started.
// If not it starts the trial period if the user has expressed intent to do so.
// If a trial is already running it validates the trial license.
func (r *ReconcileTrials) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.Tracer, name, "secret_name", request)
	defer common.LogReconciliationRun(ulog.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	log := ulog.FromContext(ctx)
	secret, license, err := licensing.TrialLicense(r, request.NamespacedName)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Trial license secret has been deleted by user, but trial had been started previously.")
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, pkgerrors.Wrap(err, "while fetching trial license")
	}

	if !license.IsECKManagedTrial() {
		// ignore externally generated licenses
		return reconcile.Result{}, nil
	}

	validationMsg := validateEULA(secret)
	if validationMsg != "" {
		return reconcile.Result{}, r.invalidOperation(ctx, secret, validationMsg)
	}

	// 1. reconcile trial status secret
	if err := r.reconcileTrialStatus(ctx, request.NamespacedName, license); err != nil {
		return reconcile.Result{}, pkgerrors.Wrap(err, "while reconciling trial status")
	}

	// 2. reconcile the trial license itself
	trialLicensePopulated := license.IsMissingFields() == nil
	licenseStatus := r.validateLicense(ctx, license)

	switch {
	case !trialLicensePopulated && r.trialState.IsTrialStarted():
		// user wants to start a trial for the second time
		return reconcile.Result{}, r.invalidOperation(ctx, secret, trialOnlyOnceMsg)
	case !trialLicensePopulated && !r.trialState.IsTrialStarted():
		// user wants to init a trial for the first time
		return reconcile.Result{}, r.initTrialLicense(ctx, secret, license)
	case trialLicensePopulated && !validLicense(licenseStatus):
		// existing license is invalid (expired or tampered with)
		return reconcile.Result{}, r.invalidOperation(ctx, secret, userFriendlyMsgs[licenseStatus])
	case trialLicensePopulated && validLicense(licenseStatus) && !r.trialState.IsTrialStarted():
		// valid license, let's consider the trial started and complete the activation
		return reconcile.Result{}, r.completeTrialActivation(ctx, request.NamespacedName)
	case trialLicensePopulated && validLicense(licenseStatus) && r.trialState.IsTrialStarted():
		// all good nothing to do
	}

	return reconcile.Result{}, err
}

func (r *ReconcileTrials) reconcileTrialStatus(ctx context.Context, licenseName types.NamespacedName, license licensing.EnterpriseLicense) error {
	var trialStatus corev1.Secret
	err := r.Get(ctx, types.NamespacedName{Namespace: r.OperatorNamespace, Name: licensing.TrialStatusSecretKey}, &trialStatus)
	if errors.IsNotFound(err) {
		if r.trialState.IsEmpty() {
			// we have no state in memory nor in the status secret: start the activation process
			if err := r.startTrialActivation(); err != nil {
				return err
			}
		}

		// we have state in memory but the status secret is missing: recreate it
		trialStatus, err = licensing.ExpectedTrialStatus(r.OperatorNamespace, licenseName, r.trialState)
		if err != nil {
			return fmt.Errorf("while creating expected trial status %w", err)
		}
		return r.Create(ctx, &trialStatus)
	}
	if err != nil {
		return fmt.Errorf("while fetching trial status %w", err)
	}

	// the status secret is there but we don't have anything in memory: recover the state
	if r.trialState.IsEmpty() {
		recoveredState, err := recoverState(license, trialStatus)
		if err != nil {
			return err
		}
		r.trialState = recoveredState
	}
	// if trial status exists, but we need to update it because:
	// - has been tampered with
	// - we need to complete the trial activation because if failed on a previous attempt
	// - we just regenerated the state after a crash
	expected, err := licensing.ExpectedTrialStatus(r.OperatorNamespace, licenseName, r.trialState)
	if err != nil {
		return err
	}
	if reflect.DeepEqual(expected.Data, trialStatus.Data) {
		return nil
	}
	trialStatus.Data = expected.Data
	return r.Update(ctx, &trialStatus)
}

func recoverState(license licensing.EnterpriseLicense, trialStatus corev1.Secret) (licensing.TrialState, error) {
	// allow new trial state only if we don't have license that looks like it has been populated previously
	allowNewState := license.IsMissingFields() != nil
	// create new keys if the operator failed just before the trial was started
	trialActivationInProgress := bytes.Equal(trialStatus.Data[licensing.TrialActivationKey], []byte("true"))
	if trialActivationInProgress && allowNewState {
		return licensing.NewTrialState()
	}
	// otherwise just recover the public key
	return licensing.NewTrialStateFromStatus(trialStatus)
}

func (r *ReconcileTrials) startTrialActivation() error {
	state, err := licensing.NewTrialState()
	if err != nil {
		return err
	}
	r.trialState = state
	return nil
}

func (r *ReconcileTrials) completeTrialActivation(ctx context.Context, license types.NamespacedName) error {
	if r.trialState.CompleteTrialActivation() {
		expectedStatus, err := licensing.ExpectedTrialStatus(r.OperatorNamespace, license, r.trialState)
		if err != nil {
			return err
		}
		_, err = reconciler.ReconcileSecret(ctx, r, expectedStatus, nil)
		return err
	}
	return nil
}

func (r *ReconcileTrials) initTrialLicense(ctx context.Context, secret corev1.Secret, license licensing.EnterpriseLicense) error {
	if err := r.trialState.InitTrialLicense(ctx, &license); err != nil {
		return err
	}
	return licensing.UpdateEnterpriseLicense(ctx, r, secret, license)
}

func (r *ReconcileTrials) invalidOperation(ctx context.Context, secret corev1.Secret, msg string) error {
	setValidationMsg(ctx, &secret, msg)
	return r.Update(ctx, &secret)
}

func validLicense(status licensing.LicenseStatus) bool {
	return status == licensing.LicenseStatusValid
}

func (r *ReconcileTrials) validateLicense(ctx context.Context, license licensing.EnterpriseLicense) licensing.LicenseStatus {
	return r.trialState.LicenseVerifier().Valid(ctx, license, time.Now())
}

func validateEULA(trialSecret corev1.Secret) string {
	if licensing.IsEnterpriseTrial(trialSecret) &&
		trialSecret.Annotations[licensing.EULAAnnotation] != licensing.EULAAcceptedValue {
		return EULAValidationMsg
	}
	return ""
}

func setValidationMsg(ctx context.Context, secret *corev1.Secret, violation string) {
	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	ulog.FromContext(ctx).Info("trial license invalid", "reason", violation)
	secret.Annotations[licensing.LicenseInvalidAnnotation] = violation
}

func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileTrials {
	return &ReconcileTrials{
		Client:     mgr.GetClient(),
		Parameters: params,
		recorder:   mgr.GetEventRecorderFor(name),
	}
}

func addWatches(mgr manager.Manager, c controller.Controller) error {
	// Watch the trial status secret and the enterprise trial licenses as well
	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{},
		handler.TypedEnqueueRequestsFromMapFunc[*corev1.Secret](func(ctx context.Context, secret *corev1.Secret) []reconcile.Request {
			if licensing.IsEnterpriseTrial(*secret) {
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Namespace: secret.GetNamespace(),
							Name:      secret.GetName(),
						},
					},
				}
			}

			if secret.GetName() != licensing.TrialStatusSecretKey {
				return nil
			}
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Namespace: secret.Annotations[licensing.TrialLicenseSecretNamespace],
						Name:      secret.Annotations[licensing.TrialLicenseSecretName],
					},
				},
			}
		}),
	))
}

// Add creates a new Trial Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	r := newReconciler(mgr, params)
	c, err := common.NewController(mgr, name, r, params)
	if err != nil {
		return err
	}
	return addWatches(mgr, c)
}

var _ reconcile.Reconciler = &ReconcileTrials{}
