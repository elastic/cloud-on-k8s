// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package beat

import (
	"context"

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

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat/auditbeat"
	beatcommon "github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat/filebeat"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat/heartbeat"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat/journalbeat"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat/metricbeat"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat/otherbeat"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat/packetbeat"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	controllerName = "beat-controller"
)

// Add creates a new Beat Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	r := newReconciler(mgr, params)
	c, err := common.NewController(mgr, controllerName, r, params)
	if err != nil {
		return err
	}
	return addWatches(mgr, c, r)
}

// newReconciler returns a new reconcile.Reconciler.
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileBeat {
	client := mgr.GetClient()
	return &ReconcileBeat{
		Client:         client,
		recorder:       mgr.GetEventRecorderFor(controllerName),
		dynamicWatches: watches.NewDynamicWatches(),
		Parameters:     params,
	}
}

// addWatches adds watches for all resources this controller cares about
func addWatches(mgr manager.Manager, c controller.Controller, r *ReconcileBeat) error {
	// Watch for changes to Beat
	if err := c.Watch(source.Kind(mgr.GetCache(), &beatv1beta1.Beat{}, &handler.TypedEnqueueRequestForObject[*beatv1beta1.Beat]{})); err != nil {
		return err
	}

	// Watch DaemonSets
	if err := c.Watch(source.Kind(mgr.GetCache(), &appsv1.DaemonSet{}, handler.TypedEnqueueRequestForOwner[*appsv1.DaemonSet](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&beatv1beta1.Beat{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}

	// Watch Deployments
	if err := c.Watch(source.Kind(mgr.GetCache(), &appsv1.Deployment{}, handler.TypedEnqueueRequestForOwner[*appsv1.Deployment](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&beatv1beta1.Beat{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}

	// Watch Pods, to ensure `status.version` is correctly reconciled on any change.
	// Watching Deployments or DaemonSets only may lead to missing some events.
	if err := watches.WatchPods(mgr, c, beatcommon.NameLabelName); err != nil {
		return err
	}

	// Watch owned and soft-owned Secrets
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, handler.TypedEnqueueRequestForOwner[*corev1.Secret](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&beatv1beta1.Beat{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}
	if err := watches.WatchSoftOwnedSecrets(mgr, c, beatv1beta1.Kind); err != nil {
		return err
	}

	// Watch dynamically referenced Secrets
	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, r.dynamicWatches.Secrets))
}

var _ reconcile.Reconciler = &ReconcileBeat{}

// ReconcileBeat reconciles a Beat object.
type ReconcileBeat struct {
	k8s.Client
	recorder       record.EventRecorder
	dynamicWatches watches.DynamicWatches
	operator.Parameters
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

// Reconcile reads that state of the cluster for a Beat object and makes changes based on the state read
// and what is in the Beat.Spec.
func (r *ReconcileBeat) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.Tracer, controllerName, "beat_name", request)
	defer common.LogReconciliationRun(ulog.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	var beat beatv1beta1.Beat
	err := r.Client.Get(ctx, request.NamespacedName, &beat)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, r.onDelete(ctx, types.NamespacedName{
				Namespace: request.Namespace,
				Name:      request.Name,
			})
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(ctx, &beat) {
		ulog.FromContext(ctx).Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", beat.Namespace, "beat_name", beat.Name)
		return reconcile.Result{}, nil
	}

	if beat.IsMarkedForDeletion() {
		return reconcile.Result{}, nil
	}

	results, status := r.doReconcile(ctx, beat)
	statusErr := beatcommon.UpdateStatus(ctx, beat, r.Client, status)
	if statusErr != nil {
		if apierrors.IsConflict(statusErr) {
			return results.WithResult(reconcile.Result{Requeue: true}).Aggregate()
		}
		results.WithError(statusErr)
	}

	res, err := results.Aggregate()
	k8s.MaybeEmitErrorEvent(r.recorder, err, &beat, events.EventReconciliationError, "Reconciliation error: %v", err)

	return res, err
}

func (r *ReconcileBeat) doReconcile(ctx context.Context, beat beatv1beta1.Beat) (*reconciler.Results, *beatv1beta1.BeatStatus) {
	results := reconciler.NewResult(ctx)
	status := newStatus(beat)

	areAssocsConfigured, err := association.AreConfiguredIfSet(ctx, beat.GetAssociations(), r.recorder)
	if err != nil {
		return results.WithError(err), &status
	}
	if !areAssocsConfigured {
		return results, &status
	}

	// Run validation in case the webhook is disabled
	if err := r.validate(ctx, &beat); err != nil {
		return results.WithError(err), &status
	}

	driverResults, updatedStatus := newDriver(ctx, r.recorder, r.Client, r.dynamicWatches, beat, status).Reconcile()
	return results.WithResults(driverResults), updatedStatus
}

func (r *ReconcileBeat) validate(ctx context.Context, beat *beatv1beta1.Beat) error {
	span, vctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	defer span.End()

	if _, err := beat.ValidateCreate(); err != nil {
		ulog.FromContext(ctx).Error(err, "Validation failed")
		k8s.MaybeEmitErrorEvent(r.recorder, err, beat, events.EventReasonValidation, err.Error())
		return tracing.CaptureError(vctx, err)
	}

	return nil
}

func (r *ReconcileBeat) onDelete(ctx context.Context, obj types.NamespacedName) error {
	r.dynamicWatches.Secrets.RemoveHandlerForKey(keystore.SecureSettingsWatchName(obj))
	r.dynamicWatches.Secrets.RemoveHandlerForKey(common.ConfigRefWatchName(obj))
	return reconciler.GarbageCollectSoftOwnedSecrets(ctx, r.Client, obj, beatv1beta1.Kind)
}

func newDriver(
	ctx context.Context,
	recorder record.EventRecorder,
	client k8s.Client,
	dynamicWatches watches.DynamicWatches,
	beat beatv1beta1.Beat,
	status beatv1beta1.BeatStatus,
) beatcommon.Driver {
	dp := beatcommon.DriverParams{
		Client:        client,
		Context:       ctx,
		Watches:       dynamicWatches,
		EventRecorder: recorder,
		Status:        &status,
		Beat:          beat,
	}

	switch beat.Spec.Type {
	case string(filebeat.Type):
		return filebeat.NewDriver(dp)
	case string(metricbeat.Type):
		return metricbeat.NewDriver(dp)
	case string(heartbeat.Type):
		return heartbeat.NewDriver(dp)
	case string(auditbeat.Type):
		return auditbeat.NewDriver(dp)
	case string(journalbeat.Type):
		return journalbeat.NewDriver(dp)
	case string(packetbeat.Type):
		return packetbeat.NewDriver(dp)
	default:
		return otherbeat.NewDriver(dp)
	}
}

// newStatus will generate a new status, ensuring status.ObservedGeneration
// follows the generation of the Beat object.
func newStatus(beat beatv1beta1.Beat) beatv1beta1.BeatStatus {
	status := beat.Status
	status.ObservedGeneration = beat.Generation
	return status
}
