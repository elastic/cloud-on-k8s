// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"reflect"
	"sync/atomic"

	kibanav1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
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

	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	name                = "kibana-controller"
	configChecksumLabel = "kibana.k8s.elastic.co/config-checksum"
)

var log = logf.Log.WithName(name)

// Add creates a new Kibana Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	reconciler := NewReconciler(mgr, params)
	c, err := add(mgr, reconciler)
	if err != nil {
		return err
	}
	return addWatches(c, reconciler)
}

// TODO sabo add kubebuilder:rbac markers?
// the example has the markers above the func (Reconciler) Reconcile()
func (r *ReconcileKibana) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&kibanav1alpha1.Kibana{}).
		Complete(r)
	if err != nil {
		return err
	}
	c, err := add(mgr, r)
	if err != nil {
		return err
	}
	return addWatches(c, r)

}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileKibana {
	client := k8s.WrapClient(mgr.GetClient())
	return &ReconcileKibana{
		Client:         client,
		scheme:         mgr.GetScheme(),
		recorder:       mgr.GetEventRecorderFor(name),
		dynamicWatches: watches.NewDynamicWatches(),
		finalizers:     finalizer.NewHandler(client),
		params:         params,
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

	params operator.Parameters

	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

// Reconcile reads that state of the cluster for a Kibana object and makes changes based on the state read and what is
// in the Kibana.Spec
func (r *ReconcileKibana) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, &r.iteration)()

	// retrieve the kibana object
	var kb kibanav1alpha1.Kibana
	if ok, err := association.FetchWithAssociation(r.Client, request, &kb); !ok {
		return reconcile.Result{}, err
	}

	// skip reconciliation if paused
	if common.IsPaused(kb.ObjectMeta) {
		log.Info("Object is paused. Skipping reconciliation", "namespace", kb.Namespace, "kibana_name", kb.Name)
		return common.PauseRequeue, nil
	}

	// check for compatibility with the operator version
	compatible, err := r.isCompatible(&kb)
	if err != nil || !compatible {
		return reconcile.Result{}, err
	}

	// run finalizers
	if err := r.finalizers.Handle(&kb, r.finalizersFor(&kb)...); err != nil {
		if errors.IsConflict(err) {
			// Conflicts are expected and should be resolved on next loop
			log.V(1).Info("Conflict while handling secret watch finalizer", "namespace", kb.Namespace, "kibana_name", kb.Name)
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, err
	}

	// Kibana will be deleted nothing to do other than run finalizers
	if kb.IsMarkedForDeletion() {
		return reconcile.Result{}, nil
	}

	// update controller version annotation if necessary
	err = annotation.UpdateControllerVersion(r.Client, &kb, r.params.OperatorInfo.BuildInfo.Version)
	if err != nil {
		return reconcile.Result{}, err
	}

	// main reconciliation logic
	return r.doReconcile(request, &kb)
}

func (r *ReconcileKibana) isCompatible(kb *kibanav1alpha1.Kibana) (bool, error) {
	selector := map[string]string{label.KibanaNameLabelName: kb.Name}
	compat, err := annotation.ReconcileCompatibility(r.Client, kb, selector, r.params.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, kb, events.EventCompatCheckError, "Error during compatibility check: %v", err)
	}

	return compat, err
}

func (r *ReconcileKibana) doReconcile(request reconcile.Request, kb *kibanav1alpha1.Kibana) (reconcile.Result, error) {
	ver, err := version.Parse(kb.Spec.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, kb, events.EventReasonValidation, "Invalid version '%s': %v", kb.Spec.Version, err)
		return reconcile.Result{}, err
	}

	state := NewState(request, kb)
	driver, err := newDriver(r, r.scheme, *ver, r.dynamicWatches, r.recorder)
	if err != nil {
		return reconcile.Result{}, err
	}
	// version specific reconcile
	results := driver.Reconcile(&state, kb, r.params)

	// update status
	err = r.updateStatus(state)
	if err != nil && errors.IsConflict(err) {
		log.V(1).Info("Conflict while updating status", "namespace", kb.Namespace, "kibana_name", kb.Name)
		return reconcile.Result{Requeue: true}, nil
	}

	res, err := results.WithError(err).Aggregate()
	k8s.EmitErrorEvent(r.recorder, err, kb, events.EventReconciliationError, "Reconciliation error: %v", err)
	return res, err
}

func (r *ReconcileKibana) updateStatus(state State) error {
	current := state.originalKibana
	if reflect.DeepEqual(current.Status, state.Kibana.Status) {
		return nil
	}
	if state.Kibana.Status.IsDegraded(current.Status) {
		r.recorder.Event(current, corev1.EventTypeWarning, events.EventReasonUnhealthy, "Kibana health degraded")
	}
	log.Info("Updating status", "iteration", atomic.LoadUint64(&r.iteration), "namespace", state.Kibana.Namespace, "kibana_name", state.Kibana.Name)
	return r.Status().Update(state.Kibana)
}

// finalizersFor returns the list of finalizers applying to a given Kibana deployment
func (r *ReconcileKibana) finalizersFor(kb *kibanav1alpha1.Kibana) []finalizer.Finalizer {
	return []finalizer.Finalizer{
		secretWatchFinalizer(*kb, r.dynamicWatches),
		keystore.Finalizer(k8s.ExtractNamespacedName(kb), r.dynamicWatches, kb.Kind()),
	}
}
