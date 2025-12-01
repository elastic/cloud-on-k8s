// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cloudconnectedmode

import (
	"context"
	"sync/atomic"
	"time"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	ccmv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/cloudconnectedmode/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	controllerName = "cloudconnectedmode-controller"
)

var (
	// defaultRequeue is the default requeue interval for this controller.
	defaultRequeue = 30 * time.Second
)

// Add creates a new CloudConnectedMode Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	r := newReconciler(mgr, params)
	c, err := common.NewController(mgr, controllerName, r, params)
	if err != nil {
		return err
	}
	return addWatches(mgr, c, r)
}

// newReconciler returns a new reconcile.Reconciler of CloudConnectedMode.
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileCloudConnectedMode {
	k8sClient := mgr.GetClient()
	return &ReconcileCloudConnectedMode{
		Client:         k8sClient,
		recorder:       mgr.GetEventRecorderFor(controllerName),
		licenseChecker: license.NewLicenseChecker(k8sClient, params.OperatorNamespace),
		params:          params,
		dynamicWatches: watches.NewDynamicWatches(),
	}
}

func addWatches(mgr manager.Manager, c controller.Controller, r *ReconcileCloudConnectedMode) error {
	// watch for changes to CloudConnectedMode
	if err := c.Watch(source.Kind(mgr.GetCache(), &ccmv1alpha1.CloudConnectedMode{}, &handler.TypedEnqueueRequestForObject[*ccmv1alpha1.CloudConnectedMode]{})); err != nil {
		return err
	}

	// watch Secrets soft owned by CloudConnectedMode
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, reconcileRequestForSoftOwner())); err != nil {
		return err
	}

	// watch dynamically referenced secrets
	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, r.dynamicWatches.Secrets))
}

func reconcileRequestForSoftOwner() handler.TypedEventHandler[*corev1.Secret, reconcile.Request] {
	return handler.TypedEnqueueRequestsFromMapFunc[*corev1.Secret](func(ctx context.Context, secret *corev1.Secret) []reconcile.Request {
		softOwner, referenced := reconciler.SoftOwnerRefFromLabels(secret.GetLabels())
		if !referenced || softOwner.Kind != ccmv1alpha1.Kind {
			return nil
		}
		return []reconcile.Request{
			{NamespacedName: types.NamespacedName{Namespace: softOwner.Namespace, Name: softOwner.Name}},
		}
	})
}

var _ reconcile.Reconciler = &ReconcileCloudConnectedMode{}

// ReconcileCloudConnectedMode reconciles a CloudConnectedMode object
type ReconcileCloudConnectedMode struct {
	k8s.Client
	recorder       record.EventRecorder
	licenseChecker license.Checker
	params         operator.Parameters
	dynamicWatches watches.DynamicWatches
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

// Reconcile reads that state of the cluster for a CloudConnectedMode object and makes changes based on the state read and what is
// in the CloudConnectedMode.Spec.
func (r *ReconcileCloudConnectedMode) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.params.Tracer, controllerName, "ccm_name", request)
	defer common.LogReconciliationRun(ulog.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	// retrieve the CloudConnectedMode resource
	var ccm ccmv1alpha1.CloudConnectedMode
	err := r.Client.Get(ctx, request.NamespacedName, &ccm)
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

	// skip unmanaged resources
	if common.IsUnmanaged(ctx, &ccm) {
		ulog.FromContext(ctx).Info("Object is currently not managed by this controller. Skipping reconciliation")
		return reconcile.Result{}, nil
	}

	// the CloudConnectedMode will be deleted nothing to do other than remove the watches
	if ccm.IsMarkedForDeletion() {
		return reconcile.Result{}, r.onDelete(ctx, k8s.ExtractNamespacedName(&ccm))
	}

	// main reconciliation logic
	results, status := r.doReconcile(ctx, ccm)

	// update status
	if err := r.updateStatus(ctx, ccm, status); err != nil {
		if apierrors.IsConflict(err) {
			return results.WithRequeue().Aggregate()
		}
		results.WithError(err)
	}

	return results.Aggregate()
}

func (r *ReconcileCloudConnectedMode) doReconcile(ctx context.Context, ccm ccmv1alpha1.CloudConnectedMode) (*reconciler.Results, ccmv1alpha1.CloudConnectedModeStatus) {
	log := ulog.FromContext(ctx)
	log.V(1).Info("Reconcile CloudConnectedMode")

	results := reconciler.NewResult(ctx)
	status := ccmv1alpha1.NewStatus(ccm)
	defer status.Update()

	// Enterprise license check
	enabled, err := r.licenseChecker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		return results.WithError(err), status
	}
	if !enabled {
		msg := "CloudConnectedMode is an enterprise feature. Enterprise features are disabled"
		log.Info(msg)
		r.recorder.Eventf(&ccm, corev1.EventTypeWarning, events.EventReconciliationError, msg)
		// we don't have a good way of watching for the license level to change so just requeue with a reasonably long delay
		return results.WithRequeue(5 * time.Minute), status
	}

	// run validation in case the webhook is disabled
	if err := r.validate(ctx, &ccm); err != nil {
		r.recorder.Eventf(&ccm, corev1.EventTypeWarning, events.EventReasonValidation, err.Error())
		return results.WithError(err), status
	}

	// TODO: Add internal reconciliation logic here
	// This is intentionally left empty as per requirements

	// requeue if not ready
	// TODO: Update this based on actual status phase when implemented
	results.WithRequeue(defaultRequeue)

	return results, status
}

func (r *ReconcileCloudConnectedMode) validate(ctx context.Context, ccm *ccmv1alpha1.CloudConnectedMode) error {
	span, vctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	defer span.End()

	if _, err := ccm.ValidateCreate(); err != nil {
		ulog.FromContext(ctx).Error(err, "Validation failed")
		k8s.MaybeEmitErrorEvent(r.recorder, err, ccm, events.EventReasonValidation, err.Error())
		return tracing.CaptureError(vctx, err)
	}

	return nil
}

func (r *ReconcileCloudConnectedMode) updateStatus(ctx context.Context, ccm ccmv1alpha1.CloudConnectedMode, status ccmv1alpha1.CloudConnectedModeStatus) error {
	span, _ := apm.StartSpan(ctx, "update_status", tracing.SpanTypeApp)
	defer span.End()

	if status.ObservedGeneration == ccm.Status.ObservedGeneration &&
		status.Resources == ccm.Status.Resources &&
		status.Ready == ccm.Status.Ready &&
		status.Errors == ccm.Status.Errors &&
		status.ReadyCount == ccm.Status.ReadyCount {
		return nil // nothing to do
	}

	ulog.FromContext(ctx).V(1).Info("Updating status",
		"iteration", atomic.LoadUint64(&r.iteration),
		"status", status,
	)
	ccm.Status = status
	return common.UpdateStatus(ctx, r.Client, &ccm)
}

func (r *ReconcileCloudConnectedMode) onDelete(ctx context.Context, obj types.NamespacedName) error {
	defer tracing.Span(&ctx)()
	// Remove dynamic watches on secrets
	// TODO: Add cleanup for any dynamic watches if needed
	return reconciler.GarbageCollectSoftOwnedSecrets(ctx, r.Client, obj, ccmv1alpha1.Kind)
}

