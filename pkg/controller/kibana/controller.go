// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"reflect"
	"sync/atomic"

	"github.com/pkg/errors"
	"go.elastic.co/apm/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	kblabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	controllerName           = "kibana-controller"
	configHashAnnotationName = "kibana.k8s.elastic.co/config-hash"
)

// Add creates a new Kibana Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	reconciler := newReconciler(mgr, params)
	c, err := common.NewController(mgr, controllerName, reconciler, params)
	if err != nil {
		return err
	}
	return addWatches(mgr, c, reconciler)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileKibana {
	return &ReconcileKibana{
		Client:         mgr.GetClient(),
		recorder:       mgr.GetEventRecorderFor(controllerName),
		dynamicWatches: watches.NewDynamicWatches(),
		params:         params,
	}
}

func addWatches(mgr manager.Manager, c controller.Controller, r *ReconcileKibana) error {
	// Watch for changes to Kibana
	if err := c.Watch(source.Kind(mgr.GetCache(), &kbv1.Kibana{}, &handler.TypedEnqueueRequestForObject[*kbv1.Kibana]{})); err != nil {
		return err
	}

	// Watch deployments
	if err := c.Watch(source.Kind(mgr.GetCache(), &appsv1.Deployment{}, handler.TypedEnqueueRequestForOwner[*appsv1.Deployment](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&kbv1.Kibana{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}

	// Watch Pods, to ensure `status.version` and version upgrades are correctly reconciled on any change.
	// Watching Deployments only may lead to missing some events.
	if err := watches.WatchPods(mgr, c, kblabel.KibanaNameLabelName); err != nil {
		return err
	}

	// Watch services
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Service{}, handler.TypedEnqueueRequestForOwner[*corev1.Service](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&kbv1.Kibana{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}
	// Watch owned and soft-owned secrets
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, handler.TypedEnqueueRequestForOwner[*corev1.Secret](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&kbv1.Kibana{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}
	if err := watches.WatchSoftOwnedSecrets(mgr, c, kbv1.Kind); err != nil {
		return err
	}

	// dynamically watch referenced secrets to connect to Elasticsearch
	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, r.dynamicWatches.Secrets))
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
func (r *ReconcileKibana) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.params.Tracer, controllerName, "kibana_name", request)
	defer common.LogReconciliationRun(ulog.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	// retrieve the kibana object
	var kb kbv1.Kibana
	err := r.Client.Get(ctx, request.NamespacedName, &kb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, r.onDelete(ctx,
				types.NamespacedName{
					Namespace: request.Namespace,
					Name:      request.Name,
				})
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(ctx, &kb) {
		ulog.FromContext(ctx).Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", kb.Namespace, "kibana_name", kb.Name)
		return reconcile.Result{}, nil
	}

	// Remove any previous Finalizers
	if err := finalizer.RemoveAll(ctx, r.Client, &kb); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// Kibana will be deleted nothing to do other than remove the watches
	if kb.IsMarkedForDeletion() {
		return reconcile.Result{}, r.onDelete(ctx, k8s.ExtractNamespacedName(&kb))
	}

	// main reconciliation logic
	return r.doReconcile(ctx, request, &kb)
}

func (r *ReconcileKibana) doReconcile(ctx context.Context, request reconcile.Request, kb *kbv1.Kibana) (result reconcile.Result, err error) {
	state := NewState(request, kb)
	log := ulog.FromContext(ctx)
	// defer the updating of status to ensure that the status is updated regardless of the outcome of the reconciliation.
	// note that this deferred function is modifying the return values, which are named return values, which allows this
	// to function properly.
	defer func() {
		statusErr := r.updateStatus(ctx, state)
		if statusErr != nil && apierrors.IsConflict(statusErr) {
			log.V(1).Info("Conflict while updating status", "namespace", kb.Namespace, "kibana_name", kb.Name)
			result = reconcile.Result{Requeue: true}
		} else if statusErr != nil {
			finalError := statusErr
			if err != nil {
				finalError = errors.Wrapf(err, "while updating status: %s", statusErr)
			}
			log.Error(finalError, "Error while updating status", "namespace", kb.Namespace, "kibana_name", kb.Name)
			err = finalError
		}
	}()

	// Run validation in case the webhook is disabled
	if err = r.validate(ctx, kb); err != nil {
		return result, err
	}

	var driver *driver
	driver, err = newDriver(r, r.dynamicWatches, r.recorder, kb, r.params.IPFamily)
	if err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	results := driver.Reconcile(ctx, &state, kb, r.params)

	result, err = results.WithError(err).Aggregate()
	k8s.MaybeEmitErrorEvent(r.recorder, err, kb, events.EventReconciliationError, "Reconciliation error: %v", err)
	return result, err
}

func (r *ReconcileKibana) validate(ctx context.Context, kb *kbv1.Kibana) error {
	span, vctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	defer span.End()

	if _, err := kb.ValidateCreate(); err != nil {
		ulog.FromContext(ctx).Error(err, "Validation failed")
		k8s.MaybeEmitErrorEvent(r.recorder, err, kb, events.EventReasonValidation, err.Error())
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
	if state.Kibana.Status.DeploymentStatus.IsDegraded(current.Status.DeploymentStatus) {
		r.recorder.Event(current, corev1.EventTypeWarning, events.EventReasonUnhealthy, "Kibana health degraded")
	}
	ulog.FromContext(ctx).V(1).Info("Updating status",
		"iteration", atomic.LoadUint64(&r.iteration),
		"namespace", state.Kibana.Namespace,
		"kibana_name", state.Kibana.Name,
		"status", state.Kibana.Status,
	)
	return common.UpdateStatus(ctx, r.Client, state.Kibana)
}

func (r *ReconcileKibana) onDelete(ctx context.Context, obj types.NamespacedName) error {
	// Clean up watches set on secure settings
	r.dynamicWatches.Secrets.RemoveHandlerForKey(keystore.SecureSettingsWatchName(obj))
	// Clean up watches set on custom http tls certificates
	r.dynamicWatches.Secrets.RemoveHandlerForKey(certificates.CertificateWatchKey(kbv1.KBNamer, obj.Name))
	return reconciler.GarbageCollectSoftOwnedSecrets(ctx, r.Client, obj, kbv1.Kind)
}

// State holds the accumulated state during the reconcile loop including the response and a pointer to a Kibana
// resource for status updates.
type State struct {
	Kibana  *kbv1.Kibana
	Request reconcile.Request

	originalKibana *kbv1.Kibana
}

// NewState creates a new reconcile state based on the given request and Kibana resource with the resource
// state reset to empty.
func NewState(request reconcile.Request, kb *kbv1.Kibana) State {
	newState := State{Request: request, Kibana: kb, originalKibana: kb.DeepCopy()}
	newState.Kibana.Status.ObservedGeneration = kb.Generation
	return newState
}
