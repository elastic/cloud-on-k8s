/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package trial

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"sync/atomic"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/events"
	licensing "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	commonvalidation "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/validation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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
	// trialMode reflects user intent to trial Enterprise features.
	trialMode bool
	// operatorNamespace is the namespace the operator is running in.
	operatorNamespace string
}

// Reconcile watches a trial status secret. If it finds a trial license it checks whether a trial has been started.
// If not it starts the trial period if the user has expressed intent to do so.
// If a trial is already running it validates the trial license.
func (r *ReconcileTrials) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration, "request", request)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime))
	}()

	licenses, err := licensing.TrialLicenses(r)
	if err != nil {
		return reconcile.Result{}, pkgerrors.Wrap(err, "while fetching trial licenses")
	}

	var license licensing.SourceEnterpriseLicense
	if len(licenses) == 0 {
		license = licensing.SourceEnterpriseLicense{
			Data: licensing.SourceLicenseData{
				Type: string(v1alpha1.LicenseTypeEnterpriseTrial),
			},
		}
	} else {
		license = licenses[0] // just take the first one
	}

	// 1. fetch trial status secret
	var trialStatus corev1.Secret
	err = r.Get(types.NamespacedName{Namespace: request.Namespace, Name: licensing.TrialStatusSecretKey}, &trialStatus)
	if errors.IsNotFound(err) {
		// 2. if not present create one + finalizer
		err := r.initTrial(request.Namespace, license)
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

func (r *ReconcileTrials) initTrial(namespace string, l licensing.SourceEnterpriseLicense) error {
	if r.isTrialRunning() {
		// silent NOOP
		return nil
	}

	trialPubKey, err := licensing.InitTrial(r, namespace, &l)
	if err != nil {
		return err
	}
	// retain pub key in memory for later iterations
	r.trialPubKey = trialPubKey
	return nil
}

func (r *ReconcileTrials) trialVerifier(trialStatus corev1.Secret) (*licensing.Verifier, error) {
	if r.isTrialRunning() {
		// prefer in memory version of the public key
		return &licensing.Verifier{
			PublicKey: r.trialPubKey,
		}, nil
	}
	// after operator restart fall back to persisted trial status
	return licensing.NewVerifier(trialStatus.Data[licensing.TrialPubkeyKey])
}

func (r *ReconcileTrials) updateStatus(l v1alpha1.EnterpriseLicense, status v1alpha1.LicenseStatus) error {
	if l.Status == status {
		// nothing to do
		return nil
	}
	log.Info("trial status update", "status", status)
	l.Status = status
	return r.Status().Update(&l)
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

func (r *ReconcileTrials) record(l v1alpha1.EnterpriseLicense, results []commonvalidation.Result) {
	for _, v := range results {
		r.recorder.Event(&l, corev1.EventTypeWarning, events.EventReasonValidation, v.Reason)
	}
}

func newReconciler(mgr manager.Manager, p operator.Parameters) *ReconcileTrials {
	return &ReconcileTrials{
		Client:            k8s.WrapClient(mgr.GetClient()),
		scheme:            mgr.GetScheme(),
		recorder:          mgr.GetRecorder(name),
		trialMode:         p.TrialMode,
		operatorNamespace: p.OperatorNamespace,
	}
}

func add(mgr manager.Manager, r *ReconcileTrials) error {
	// Create a new controller
	c, err := controller.New(name, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch the trial status secret
	evtHandler := &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(obj handler.MapObject) []reconcile.Request {
			if obj.Meta.GetName() != licensing.TrialStatusSecretKey {
				return nil
			}
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Namespace: obj.Meta.GetNamespace(),
						Name:      licensing.TrialStatusSecretKey,
					},
				},
			}
		}),
	}
	// TODO introduce config file and watch that. For now just bootstrap the controller with a generic event based on flag
	configSrc := func(h handler.EventHandler, q workqueue.RateLimitingInterface, _ ...predicate.Predicate) error {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: r.operatorNamespace,
				Name:      licensing.TrialStatusSecretKey,
			},
		})
		return nil

	}

	if err := c.Watch(source.Func(configSrc), evtHandler); err != nil {
		return err
	}

	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, evtHandler); err != nil {
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
