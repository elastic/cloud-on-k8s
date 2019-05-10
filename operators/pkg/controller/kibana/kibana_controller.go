// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"reflect"
	"sync/atomic"
	"time"

	kibanav1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	name                = "kibana-controller"
	configChecksumLabel = "kibana.k8s.elastic.co/config-checksum"
)

var log = logf.Log.WithName(name)

// Add creates a new Kibana Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, _ operator.Parameters) error {
	reconciler := newReconciler(mgr)
	c, err := add(mgr, reconciler)
	if err != nil {
		return err
	}
	return addWatches(c, reconciler)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileKibana {
	client := k8s.WrapClient(mgr.GetClient())
	return &ReconcileKibana{
		Client:         client,
		scheme:         mgr.GetScheme(),
		recorder:       mgr.GetRecorder(name),
		dynamicWatches: watches.NewDynamicWatches(),
		finalizers:     finalizer.NewHandler(client),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) (controller.Controller, error) {
	// Create a new controller
	return controller.New(name, mgr, controller.Options{Reconciler: r})
}

func addWatches(c controller.Controller, r *ReconcileKibana) error {
	// Watch for changes to Kibana
	if err := c.Watch(&source.Kind{Type: &kibanav1alpha1.Kibana{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch deployments
	if err := c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kibanav1alpha1.Kibana{},
	}); err != nil {
		return err
	}

	// Watch services
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kibanav1alpha1.Kibana{},
	}); err != nil {
		return err
	}

	// Watch secrets
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kibanav1alpha1.Kibana{},
	}); err != nil {
		return err
	}

	// dynamically watch referenced secrets to connect to Elasticsearch
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, r.dynamicWatches.Secrets); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileKibana{}

// ReconcileKibana reconciles a Kibana object
type ReconcileKibana struct {
	k8s.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder

	finalizers     finalizer.Handler
	dynamicWatches watches.DynamicWatches

	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}

// Reconcile reads that state of the cluster for a Kibana object and makes changes based on the state read and what is
// in the Kibana.Spec
func (r *ReconcileKibana) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime))
	}()

	// Fetch the Kibana instance
	kb := &kibanav1alpha1.Kibana{}
	err := r.Get(request.NamespacedName, kb)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if common.IsPaused(kb.ObjectMeta) {
		log.Info("Paused : skipping reconciliation", "iteration", currentIteration)
		return common.PauseRequeue, nil
	}

	if err := r.finalizers.Handle(kb, secretWatchFinalizer(*kb, r.dynamicWatches)); err != nil {
		if errors.IsConflict(err) {
			log.V(1).Info("Conflict while handling secret watch finalizer")
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, err
	}

	if kb.IsMarkedForDeletion() {
		// Kibana will be deleted nothing to do other than run finalizers
		return reconcile.Result{}, nil
	}

	ver, err := version.Parse(kb.Spec.Version)
	if err != nil {
		return reconcile.Result{}, err
	}
	state := NewState(request, kb)
	driver, err := newDriver(r, r.scheme, *ver, r.dynamicWatches)
	if err != nil {
		return reconcile.Result{}, err
	}
	// version specific reconcile
	results := driver.Reconcile(&state, kb)
	// update status
	err = r.updateStatus(state)
	if err != nil && errors.IsConflict(err) {
		log.V(1).Info("Conflict while updating status")
		return reconcile.Result{Requeue: true}, nil
	}
	return results.WithError(err).Aggregate()
}

func (r *ReconcileKibana) updateStatus(state State) error {
	current := state.originalKibana
	if reflect.DeepEqual(current.Status, state.Kibana.Status) {
		return nil
	}
	if state.Kibana.Status.IsDegraded(current.Status) {
		r.recorder.Event(current, corev1.EventTypeWarning, events.EventReasonUnhealthy, "Kibana health degraded")
	}
	log.Info("Updating status", "iteration", atomic.LoadInt64(&r.iteration))
	return r.Status().Update(state.Kibana)
}
