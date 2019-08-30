// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package trial

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"sync/atomic"
	"time"

	licensing "github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/license/validation"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	name = "trial-controller"
)

var (
	log = logf.Log.WithName(name)
)

// ReconcileTrials reconciles Enterprise trial licenses.
type ReconcileTrials struct {
	k8s.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	// iteration is the number of times this controller has run its Reconcile method.
	iteration   int64
	trialPubKey *rsa.PublicKey
}

// Aggregate came from the webhook trial license pkg which was removed
// TODO (sabo) what do?
// func Aggregate(results []commonvalidation.Result) types.Response {
// 	response := commonvalidation.Result{Allowed: true}
// 	for _, r := range results {
// 		if !r.Allowed {
// 			response.Allowed = false
// 			if r.Error != nil {
// 				log.Error(r.Error, r.Reason)
// 			}
// 			if response.Reason == "" {
// 				response.Reason = r.Reason
// 				continue
// 			}
// 			response.Reason = response.Reason + ". " + r.Reason
// 		}
// 	}
// 	log.V(1).Info("Admission validation response", "allowed", response.Allowed, "reason", response.Reason)
// 	return admission.ValidationResponse(response.Allowed, response.Reason)
// }

// Reconcile watches a trial status secret. If it finds a trial license it checks whether a trial has been started.
// If not it starts the trial period if the user has expressed intent to do so.
// If a trial is already running it validates the trial license.
func (r *ReconcileTrials) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration, "namespace", request.Namespace, "secret_name", request.Name)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime), "namespace", request.Namespace, "secret_name", request.Name)
	}()

	secret, license, err := licensing.TrialLicense(r, request.NamespacedName)
	if err != nil {
		return reconcile.Result{}, pkgerrors.Wrap(err, "while fetching trial license")
	}

	violations := validation.Validate(secret)
	if len(violations) > 0 {
		if secret.Annotations == nil {
			secret.Annotations = map[string]string{}
		}
		// TODO (sabo) what do? this is the only call
		// res := license_validation.Aggregate(violations)
		// res := Aggregate(violations)
		// secret.Annotations[licensing.LicenseInvalidAnnotation] = string(res.Response.Result.Reason)
		return reconcile.Result{}, licensing.UpdateEnterpriseLicense(r, secret, license)
	}

	// 1. fetch trial status secret
	var trialStatus corev1.Secret
	err = r.Get(types.NamespacedName{Namespace: request.Namespace, Name: licensing.TrialStatusSecretKey}, &trialStatus)
	if errors.IsNotFound(err) {
		// 2. if not present create one + finalizer
		err := r.initTrial(secret, license)
		if err != nil {
			return reconcile.Result{}, pkgerrors.Wrap(err, "failed to init trial")
		}
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, pkgerrors.Wrap(err, "failed to retrieve trial status")
	}
	// 3. reconcile trial status
	if err := r.reconcileTrialStatus(trialStatus); err != nil {
		return reconcile.Result{}, pkgerrors.Wrap(err, "failed to reconcile trial status")
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileTrials) isTrialRunning() bool {
	return r.trialPubKey != nil
}

func (r *ReconcileTrials) initTrial(secret corev1.Secret, l licensing.EnterpriseLicense) error {
	if r.isTrialRunning() {
		// silent NOOP
		return nil
	}

	trialPubKey, err := licensing.InitTrial(r, secret, &l)
	if err != nil {
		return err
	}
	// retain pub key in memory for later iterations
	r.trialPubKey = trialPubKey
	return nil
}

func (r *ReconcileTrials) reconcileTrialStatus(trialStatus corev1.Secret) error {
	if !r.isTrialRunning() {
		return nil
	}
	pubkeyBytes, err := x509.MarshalPKIXPublicKey(r.trialPubKey)
	if err != nil {
		return err
	}
	if bytes.Equal(trialStatus.Data[licensing.TrialPubkeyKey], pubkeyBytes) {
		return nil
	}
	trialStatus.Data[licensing.TrialPubkeyKey] = pubkeyBytes
	return r.Update(&trialStatus)

}

func newReconciler(mgr manager.Manager, _ operator.Parameters) *ReconcileTrials {
	return &ReconcileTrials{
		Client:   k8s.WrapClient(mgr.GetClient()),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(name),
	}
}

func add(mgr manager.Manager, r *ReconcileTrials) error {
	// Create a new controller
	c, err := controller.New(name, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch the trial status secret and the enterprise trial licenses as well
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(obj handler.MapObject) []reconcile.Request {
			secret, ok := obj.Object.(*corev1.Secret)
			if !ok {
				log.Error(fmt.Errorf("object of type %T in secret watch", obj.Object), "dropping event due to type error")
			}
			if licensing.IsEnterpriseTrial(*secret) {
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Namespace: obj.Meta.GetNamespace(),
							Name:      obj.Meta.GetName(),
						},
					},
				}
			}

			if obj.Meta.GetName() != licensing.TrialStatusSecretKey {
				return nil
			}
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Namespace: obj.Meta.GetNamespace(),
						Name:      string(licensing.LicenseTypeEnterpriseTrial),
					},
				},
			}
		}),
	}); err != nil {
		return err
	}
	return nil
}

// Add creates a new Trial Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, p operator.Parameters) error {
	r := newReconciler(mgr, p)
	return add(mgr, r)
}

var _ reconcile.Reconciler = &ReconcileTrials{}
