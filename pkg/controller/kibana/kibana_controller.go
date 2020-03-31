// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"context"
	"reflect"
	"sync/atomic"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"go.elastic.co/apm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	name                = "kibana-controller"
	configChecksumLabel = "kibana.k8s.elastic.co/config-checksum"
)

var log = logf.Log.WithName(name)

// Add creates a new Kibana Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	reconciler := newReconciler(mgr, params)
	c, err := common.NewController(mgr, name, reconciler, params)
	if err != nil {
		return err
	}
	return addWatches(c, reconciler)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileKibana {
	client := k8s.WrapClient(mgr.GetClient())
	return &ReconcileKibana{
		Client:         client,
		recorder:       mgr.GetEventRecorderFor(name),
		dynamicWatches: watches.NewDynamicWatches(),
		params:         params,
	}
}

func addWatches(c controller.Controller, r *ReconcileKibana) error {
	// Watch for changes to Kibana
	if err := c.Watch(&source.Kind{Type: &kbv1.Kibana{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch deployments
	if err := c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kbv1.Kibana{},
	}); err != nil {
		return err
	}

	// Watch services
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kbv1.Kibana{},
	}); err != nil {
		return err
	}

	// Watch secrets
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kbv1.Kibana{},
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
	recorder record.EventRecorder

	dynamicWatches watches.DynamicWatches

	params operator.Parameters

	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

// Reconcile reads that state of the cluster for a Kibana object and makes changes based on the state read and what is
// in the Kibana.Spec
func (r *ReconcileKibana) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, "kibana_name", &r.iteration)()
	tx, ctx := tracing.NewTransaction(r.params.Tracer, request.NamespacedName, "kibana")
	defer tracing.EndTransaction(tx)

	// retrieve the kibana object
	var kb kbv1.Kibana
	if err := association.FetchWithAssociation(ctx, r.Client, request, &kb); err != nil {
		if apierrors.IsNotFound(err) {
			r.onDelete(types.NamespacedName{
				Namespace: request.Namespace,
				Name:      request.Name,
			})
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(kb.ObjectMeta) {
		log.Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", kb.Namespace, "kibana_name", kb.Name)
		return reconcile.Result{}, nil
	}

	// check for compatibility with the operator version
	compatible, err := r.isCompatible(ctx, &kb)
	if err != nil || !compatible {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// Remove any previous Finalizers
	if err := finalizer.RemoveAll(r.Client, &kb); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// Kibana will be deleted nothing to do other than remove the watches
	if kb.IsMarkedForDeletion() {
		r.onDelete(k8s.ExtractNamespacedName(&kb))
		return reconcile.Result{}, nil
	}

	// update controller version annotation if necessary
	err = annotation.UpdateControllerVersion(ctx, r.Client, &kb, r.params.OperatorInfo.BuildInfo.Version)
	if err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// main reconciliation logic
	return r.doReconcile(ctx, request, &kb)
}

func (r *ReconcileKibana) isCompatible(ctx context.Context, kb *kbv1.Kibana) (bool, error) {
	selector := map[string]string{label.KibanaNameLabelName: kb.Name}
	compat, err := annotation.ReconcileCompatibility(ctx, r.Client, kb, selector, r.params.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, kb, events.EventCompatCheckError, "Error during compatibility check: %v", err)
	}
	return compat, err
}

func (r *ReconcileKibana) doReconcile(ctx context.Context, request reconcile.Request, kb *kbv1.Kibana) (reconcile.Result, error) {
	// Run validation in case the webhook is disabled
	if err := r.validate(ctx, kb); err != nil {
		return reconcile.Result{}, err
	}

	driver, err := newDriver(r, r.dynamicWatches, r.recorder, kb)
	if err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	state := NewState(request, kb)
	results := driver.Reconcile(ctx, &state, kb, r.params)

	// update status
	err = r.updateStatus(ctx, state)
	if err != nil && apierrors.IsConflict(err) {
		log.V(1).Info("Conflict while updating status", "namespace", kb.Namespace, "kibana_name", kb.Name)
		return reconcile.Result{Requeue: true}, nil
	}

	res, err := results.WithError(err).Aggregate()
	k8s.EmitErrorEvent(r.recorder, err, kb, events.EventReconciliationError, "Reconciliation error: %v", err)
	return res, err
}

func (r *ReconcileKibana) validate(ctx context.Context, kb *kbv1.Kibana) error {
	span, vctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	defer span.End()

	if err := kb.ValidateCreate(); err != nil {
		log.Error(err, "Validation failed")
		k8s.EmitErrorEvent(r.recorder, err, kb, events.EventReasonValidation, err.Error())
		return tracing.CaptureError(vctx, err)
	}

	return nil
}

func (r *ReconcileKibana) updateStatus(ctx context.Context, state State) error {
	span, _ := apm.StartSpan(ctx, "update_status", tracing.SpanTypeApp)
	defer span.End()

	current := state.originalKibana
	if reflect.DeepEqual(current.Status, state.Kibana.Status) {
		return nil
	}
	if state.Kibana.Status.IsDegraded(current.Status) {
		r.recorder.Event(current, corev1.EventTypeWarning, events.EventReasonUnhealthy, "Kibana health degraded")
	}
	log.V(1).Info("Updating status",
		"iteration", atomic.LoadUint64(&r.iteration),
		"namespace", state.Kibana.Namespace,
		"kibana_name", state.Kibana.Name,
		"status", state.Kibana.Status,
	)
	return common.UpdateStatus(r.Client, state.Kibana)
}

func (r *ReconcileKibana) onDelete(obj types.NamespacedName) {
	// Clean up watches set on secure settings
	r.dynamicWatches.Secrets.RemoveHandlerForKey(keystore.SecureSettingsWatchName(obj))
}
