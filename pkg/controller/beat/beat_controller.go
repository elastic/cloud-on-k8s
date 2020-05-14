// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"context"

	"go.elastic.co/apm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	commonbeat "github.com/elastic/cloud-on-k8s/pkg/controller/common/beat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/beat/filebeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/beat/otherbeat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	controllerName = "beat-controller"
)

var log = logf.Log.WithName(controllerName)

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

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileBeat {
	client := k8s.WrapClient(mgr.GetClient())
	return &ReconcileBeat{
		Client:         client,
		recorder:       mgr.GetEventRecorderFor(controllerName),
		dynamicWatches: watches.NewDynamicWatches(),
		Parameters:     params,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func addWatches(c controller.Controller, r *ReconcileBeat) error {
	// Watch for changes to Beat
	if err := c.Watch(&source.Kind{Type: &beatv1beta1.Beat{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	if err := c.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &beatv1beta1.Beat{},
	}); err != nil {
		return err
	}

	if err := c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &beatv1beta1.Beat{},
	}); err != nil {
		return err
	}

	// Watch secrets
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, r.dynamicWatches.Secrets); err != nil {
		return err
	}
	if err := r.dynamicWatches.Secrets.AddHandler(&watches.OwnerWatch{
		EnqueueRequestForOwner: handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &beatv1beta1.Beat{},
		},
	}); err != nil {
		return err
	}

	if commonbeat.ShouldSetupAutodiscoverRBAC() {
		if err := c.Watch(&source.Kind{Type: &corev1.ServiceAccount{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &beatv1beta1.Beat{},
		}); err != nil {
			return err
		}

		if err := c.Watch(&source.Kind{Type: &rbacv1.ClusterRole{}}, &handler.EnqueueRequestForObject{}); err != nil {
			return err
		}

		if err := c.Watch(&source.Kind{Type: &rbacv1.ClusterRoleBinding{}}, &handler.EnqueueRequestForObject{}); err != nil {
			return err
		}
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileBeat{}

// ReconcileBeat reconciles a Beat object
type ReconcileBeat struct {
	k8s.Client
	recorder       record.EventRecorder
	dynamicWatches watches.DynamicWatches
	operator.Parameters
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

// Reconcile reads that state of the cluster for a Beat object and makes changes based on the state read
// and what is in the Beat.Spec
func (r *ReconcileBeat) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, "beat_name", &r.iteration)()
	tx, ctx := tracing.NewTransaction(r.Tracer, request.NamespacedName, "beat")
	defer tracing.EndTransaction(tx)

	var beat beatv1beta1.Beat
	if err := association.FetchWithAssociation(ctx, r.Client, request, &beat); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(&beat) {
		log.Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", beat.Namespace, "ent_name", beat.Name)
		return reconcile.Result{}, nil
	}

	if compatible, err := r.isCompatible(ctx, &beat); err != nil || !compatible {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if beat.IsMarkedForDeletion() {
		return reconcile.Result{}, nil
	}

	if err := annotation.UpdateControllerVersion(ctx, r.Client, &beat, r.OperatorInfo.BuildInfo.Version); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	res, err := r.doReconcile(ctx, beat).Aggregate()
	k8s.EmitErrorEvent(r.recorder, err, &beat, events.EventReconciliationError, "Reconciliation error: %v", err)

	return res, err
}

func (r *ReconcileBeat) doReconcile(ctx context.Context, beat beatv1beta1.Beat) *reconciler.Results {
	results := reconciler.NewResult(ctx)
	if !association.IsConfiguredIfSet(&beat, r.recorder) {
		return results
	}

	// Run validation in case the webhook is disabled
	if err := r.validate(ctx, &beat); err != nil {
		return results.WithError(err)
	}

	result := newDriver(ctx, r.Client, beat).Reconcile()
	results.WithResults(result.Results)

	err := r.updateStatus(result.Status, beat)
	results.WithError(err)
	if err != nil && apierrors.IsConflict(err) {
		log.V(1).Info("Conflict while updating status", "namespace", beat.Namespace, "beat_name", beat.Name)
		results.WithResult(reconcile.Result{Requeue: true})
	}

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

func (r *ReconcileBeat) updateStatus(driverStatus *commonbeat.DriverStatus, beat beatv1beta1.Beat) error {
	if driverStatus == nil {
		return nil
	}

	beat.Status.AvailableNodes = driverStatus.AvailableNodes
	beat.Status.ExpectedNodes = driverStatus.ExpectedNodes
	beat.Status.Health = driverStatus.Health
	beat.Status.Association = driverStatus.Association

	return r.Client.Status().Update(&beat)
}

func (r *ReconcileBeat) isCompatible(ctx context.Context, beat *beatv1beta1.Beat) (bool, error) {
	selector := map[string]string{NameLabelName: beat.Name}
	compat, err := annotation.ReconcileCompatibility(ctx, r.Client, beat, selector, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, beat, events.EventCompatCheckError, "Error during compatibility check: %v", err)
	}
	return compat, err
}

func newDriver(ctx context.Context, client k8s.Client, beat beatv1beta1.Beat) commonbeat.Driver {
	dp := newDriverParams(ctx, client, beat)

	switch dp.Type {
	case string(filebeat.Type):
		return filebeat.NewDriver(dp)
	default:
		return otherbeat.NewDriver(dp)
	}
}

func newDriverParams(ctx context.Context, client k8s.Client, beat beatv1beta1.Beat) commonbeat.DriverParams {
	spec := beat.Spec

	var ds commonbeat.DaemonSetSpec
	if spec.DaemonSet != nil {
		ds = commonbeat.DaemonSetSpec{PodTemplate: spec.DaemonSet.PodTemplate}
	}
	var d commonbeat.DeploymentSpec
	if spec.Deployment != nil {
		d = commonbeat.DeploymentSpec{Replicas: spec.Deployment.Replicas, PodTemplate: spec.Deployment.PodTemplate}
	}

	return commonbeat.DriverParams{
		Client:  client,
		Context: ctx,
		Logger:  log,

		Namer:      &Namer{},
		Owner:      &beat,
		Associated: &beat,

		Type:               spec.Type,
		Version:            spec.Version,
		ElasticsearchRef:   spec.ElasticsearchRef,
		Image:              spec.Image,
		Config:             spec.Config,
		ServiceAccountName: spec.ServiceAccountName,

		DaemonSet:  ds,
		Deployment: d,
		Labels: map[string]string{
			common.TypeLabelName: Type,
			NameLabelName:        beat.Name,
		},
		Selectors: map[string]string{
			common.TypeLabelName: Type,
			NameLabelName:        beat.Name,
		},
	}
}
