// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package beat

import (
	"context"

	"go.elastic.co/apm"
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

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/auditbeat"
	beatcommon "github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/filebeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/heartbeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/journalbeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/metricbeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/otherbeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/packetbeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
)

const (
	controllerName = "beat-controller"
)

var log = ulog.Log.WithName(controllerName)

// Add creates a new Beat Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	r := newReconciler(mgr, params)
	c, err := common.NewController(mgr, controllerName, r, params)
	if err != nil {
		return err
	}
	return addWatches(c, r)
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
func addWatches(c controller.Controller, r *ReconcileBeat) error {
	// Watch for changes to Beat
	if err := c.Watch(&source.Kind{Type: &beatv1beta1.Beat{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch DaemonSets
	if err := c.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &beatv1beta1.Beat{},
	}); err != nil {
		return err
	}

	// Watch Deployments
	if err := c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &beatv1beta1.Beat{},
	}); err != nil {
		return err
	}

	// Watch Pods, to ensure `status.version` is correctly reconciled on any change.
	// Watching Deployments or DaemonSets only may lead to missing some events.
	if err := watches.WatchPods(c, beatcommon.NameLabelName); err != nil {
		return err
	}

	// Watch owned and soft-owned Secrets
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &beatv1beta1.Beat{},
	}); err != nil {
		return err
	}
	if err := watches.WatchSoftOwnedSecrets(c, beatv1beta1.Kind); err != nil {
		return err
	}

	// Watch dynamically referenced Secrets
	return c.Watch(&source.Kind{Type: &corev1.Secret{}}, r.dynamicWatches.Secrets)
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
	defer common.LogReconciliationRun(log, request, "beat_name", &r.iteration)()
	tx, ctx := tracing.NewTransaction(ctx, r.Tracer, request.NamespacedName, "beat")
	defer tracing.EndTransaction(tx)

	var beat beatv1beta1.Beat
	err := r.Client.Get(ctx, request.NamespacedName, &beat)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, r.onDelete(types.NamespacedName{
				Namespace: request.Namespace,
				Name:      request.Name,
			})
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(&beat) {
		log.Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", beat.Namespace, "beat_name", beat.Name)
		return reconcile.Result{}, nil
	}

	if beat.IsMarkedForDeletion() {
		return reconcile.Result{}, nil
	}

	res, err := r.doReconcile(ctx, beat).Aggregate()
	k8s.EmitErrorEvent(r.recorder, err, &beat, events.EventReconciliationError, "Reconciliation error: %v", err)

	return res, err
}

func (r *ReconcileBeat) doReconcile(ctx context.Context, beat beatv1beta1.Beat) *reconciler.Results {
	results := reconciler.NewResult(ctx)
	areAssocsConfigured, err := association.AreConfiguredIfSet(beat.GetAssociations(), r.recorder)
	if err != nil {
		return results.WithError(err)
	}
	if !areAssocsConfigured {
		return results
	}

	// Run validation in case the webhook is disabled
	if err := r.validate(ctx, &beat); err != nil {
		return results.WithError(err)
	}

	driverResults := newDriver(ctx, r.recorder, r.Client, r.dynamicWatches, beat).Reconcile()
	results.WithResults(driverResults)

	return results
}

func (r *ReconcileBeat) validate(ctx context.Context, beat *beatv1beta1.Beat) error {
	span, vctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	defer span.End()

	if err := beat.ValidateCreate(); err != nil {
		log.Error(err, "Validation failed")
		k8s.EmitErrorEvent(r.recorder, err, beat, events.EventReasonValidation, err.Error())
		return tracing.CaptureError(vctx, err)
	}

	return nil
}

func (r *ReconcileBeat) onDelete(obj types.NamespacedName) error {
	r.dynamicWatches.Secrets.RemoveHandlerForKey(keystore.SecureSettingsWatchName(obj))
	r.dynamicWatches.Secrets.RemoveHandlerForKey(common.ConfigRefWatchName(obj))
	return reconciler.GarbageCollectSoftOwnedSecrets(r.Client, obj, beatv1beta1.Kind)
}

func newDriver(
	ctx context.Context,
	recorder record.EventRecorder,
	client k8s.Client,
	dynamicWatches watches.DynamicWatches,
	beat beatv1beta1.Beat,
) beatcommon.Driver {
	dp := beatcommon.DriverParams{
		Client:        client,
		Context:       ctx,
		Logger:        log,
		Watches:       dynamicWatches,
		EventRecorder: recorder,
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
