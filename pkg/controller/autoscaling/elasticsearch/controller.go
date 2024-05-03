// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	autoscalingv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/autoscaling/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/autoscaling/elasticsearch/status"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/autoscaling/elasticsearch/validation"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	commonesclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/esclient"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	logconf "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/net"
)

type EsClientProvider func(ctx context.Context, c k8s.Client, dialer net.Dialer, es esv1.Elasticsearch) (esclient.Client, error)

const (
	// ControllerName is the name of the autoscaling controller based on the dedicated Elasticsearch autoscaling resource. It supersedes the legacy
	// controller which is reading the autoscaling specification in an annotation on the Elasticsearch resource.
	ControllerName = "elasticsearch-autoscaler"

	enterpriseFeaturesDisabledMsg = "Autoscaling is an enterprise feature. Enterprise features are disabled"
)

// licenseCheckRequeue is the default duration used to retry a licence check if the cluster is supposed to be managed by
// the autoscaling controller and if the licence is not valid.
var licenseCheckRequeue = reconcile.Result{
	Requeue:      true,
	RequeueAfter: 60 * time.Second,
}

// baseReconcileAutoscaling is the base struct for both the legacy and the CRD based reconcilers.
type baseReconcileAutoscaling struct {
	k8s.Client
	operator.Parameters
	esClientProvider EsClientProvider
	recorder         record.EventRecorder
	licenseChecker   license.Checker

	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64 //nolint:structcheck
}

func (r baseReconcileAutoscaling) withRecorder(recorder record.EventRecorder) baseReconcileAutoscaling {
	r.recorder = recorder
	return r
}

// ReconcileElasticsearchAutoscaler reconciles autoscaling policies and Elasticsearch resources specifications based on
// Elasticsearch autoscaling API response.
type ReconcileElasticsearchAutoscaler struct {
	baseReconcileAutoscaling
	Watches watches.DynamicWatches
}

// NewReconciler returns a new autoscaling reconcile.Reconciler
func NewReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileElasticsearchAutoscaler {
	c := mgr.GetClient()
	reconcileAutoscaling := baseReconcileAutoscaling{
		Client:           c,
		Parameters:       params,
		esClientProvider: commonesclient.NewClient,
		recorder:         mgr.GetEventRecorderFor(ControllerName),
		licenseChecker:   license.NewLicenseChecker(c, params.OperatorNamespace),
	}
	return &ReconcileElasticsearchAutoscaler{
		baseReconcileAutoscaling: reconcileAutoscaling.withRecorder(mgr.GetEventRecorderFor(ControllerName)),
		Watches:                  watches.NewDynamicWatches(),
	}
}

func dynamicWatchName(request reconcile.Request) string {
	return fmt.Sprintf("%s-%s-referenced-es-watch", request.Namespace, request.Name)
}

func (r *ReconcileElasticsearchAutoscaler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.Tracer, ControllerName, "esa_name", request)
	defer common.LogReconciliationRun(logconf.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	log := logconf.FromContext(ctx)
	// Fetch the ElasticsearchAutoscaler instance
	var esa autoscalingv1alpha1.ElasticsearchAutoscaler
	if err := r.Get(ctx, request.NamespacedName, &esa); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("ElasticsearchAutoscaler not found", "namespace", request.Namespace, "esa_name", request.Name)
			r.Watches.ReferencedResources.RemoveHandlerForKey(dynamicWatchName(request))
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// Ensure we watch the associated Elasticsearch
	esNamespacedName := types.NamespacedName{Name: esa.Spec.ElasticsearchRef.Name, Namespace: request.Namespace}
	if err := r.Watches.ReferencedResources.AddHandler(watches.NamedWatch[client.Object]{
		Name:    dynamicWatchName(request),
		Watched: []types.NamespacedName{esNamespacedName},
		Watcher: request.NamespacedName,
	}); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(ctx, &esa) {
		msg := "Object is currently not managed by this controller. Skipping reconciliation"
		log.Info(msg, "namespace", request.Namespace, "esa_name", request.Name)
		return r.reportAsInactive(ctx, log, esa, msg)
	}

	enabled, err := r.licenseChecker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !enabled {
		log.Info(enterpriseFeaturesDisabledMsg)
		r.recorder.Eventf(&esa, corev1.EventTypeWarning, license.EventInvalidLicense, enterpriseFeaturesDisabledMsg)
		_, err := r.reportAsInactive(ctx, log, esa, enterpriseFeaturesDisabledMsg)
		// We still schedule a reconciliation in case a valid license is applied later
		return licenseCheckRequeue, err
	}

	// Fetch the Elasticsearch resource
	var es esv1.Elasticsearch
	if err := r.Get(ctx, esNamespacedName, &es); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("Elasticsearch resource %s/%s not found", esNamespacedName.Namespace, esNamespacedName.Name)
			log.Info(msg, "namespace", request.Namespace, "esa_name", request.Name, "es_name", esNamespacedName.Name, "error", err.Error())
			return r.reportAsInactive(ctx, log, esa, msg)
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// Validate the autoscaling specification
	if validationErr, runtimeErr := validation.ValidateElasticsearchAutoscaler(ctx, r.Client, esa, r.licenseChecker); validationErr != nil || runtimeErr != nil {
		if validationErr != nil {
			log.Error(
				validationErr,
				"ElasticsearchAutoscaler manifest validation failed",
				"namespace", es.Namespace,
				"esa_name", es.Name,
			)
		}
		if runtimeErr != nil {
			log.Error(
				runtimeErr,
				"Runtime error while validating ElasticsearchAutoscaler manifest",
				"namespace", es.Namespace,
				"esa_name", es.Name,
			)
		}
		err := errors.NewAggregate([]error{validationErr, runtimeErr})
		_, _ = r.reportAsUnhealthy(ctx, log, esa, err.Error())
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// Get autoscaling policies and the associated node sets.
	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		return reconcile.Result{}, err
	}
	autoscaledNodeSets, nodeSetErr := es.GetAutoscaledNodeSets(v, esa.Spec.AutoscalingPolicySpecs)
	if nodeSetErr != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, nodeSetErr)
	}
	log.V(1).Info(
		"Autoscaling policies and node sets",
		"policies", autoscaledNodeSets.Names(),
		"namespace", request.Namespace,
		"esa_name", request.Name,
	)

	// Import existing resources in the current Status if the cluster is managed by some autoscaling policies but
	// the status annotation does not exist.
	if err := status.ImportExistingResources(
		log,
		r.Client,
		esa.Spec.AutoscalingPolicySpecs,
		es,
		autoscaledNodeSets,
		&esa.Status,
	); err != nil {
		_, _ = r.reportAsUnhealthy(ctx, log, esa, fmt.Sprintf("error while importing resources from the status subresource: %s", err.Error()))
		// Status is updated on a best effort basis, we don't really care if the error above is not reported.
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	statusBuilder := newStatusBuilder(log, esa.Spec.AutoscalingPolicySpecs)
	results := &reconciler.Results{}

	// Call the main function
	reconciledEs, reconcileInternalErr := r.reconcileInternal(ctx, es, statusBuilder, autoscaledNodeSets, &esa)
	if reconcileInternalErr != nil {
		// we do not return immediately as not all errors prevent to compute a reconciled Elasticsearch resource.
		results.WithError(reconcileInternalErr)
	}

	// Update the new status
	newStatus := statusBuilder.Build()
	esa.Status.ObservedGeneration = ptr.To[int64](esa.Generation)
	esa.Status.Conditions = esa.Status.Conditions.MergeWith(newStatus.Conditions...)
	esa.Status.AutoscalingPolicyStatuses = newStatus.AutoscalingPolicyStatuses
	updateStatus, err := r.updateStatus(ctx, log, esa)
	if err != nil {
		return reconcile.Result{}, err
	}
	results.WithResult(updateStatus)

	if reconciledEs == nil {
		// No Elasticsearch resource, with up-to-date compute and storage resources, has been returned.
		// It's likely to be the case if a fatal error prevented resource calculation from the Elasticsearch
		// autoscaling API or if an "offline" reconciliation failed.
		return results.Aggregate()
	}

	// Update the Elasticsearch resource
	if err := r.Client.Update(ctx, reconciledEs); err != nil {
		if apierrors.IsConflict(err) {
			return results.WithResult(reconcile.Result{Requeue: true}).Aggregate()
		}
		return results.WithError(err).Aggregate()
	}
	return results.WithResults(defaultResult(&esa)).Aggregate()
}

// reportAsUnhealthy reports the autoscaler as inactive in the status.
func (r *ReconcileElasticsearchAutoscaler) reportAsUnhealthy(
	ctx context.Context,
	log logr.Logger,
	esa autoscalingv1alpha1.ElasticsearchAutoscaler,
	message string,
) (reconcile.Result, error) {
	now := metav1.Now()
	newStatus := esa.Status.DeepCopy()
	newStatus.ObservedGeneration = ptr.To[int64](esa.Generation)
	newStatus.Conditions = newStatus.Conditions.MergeWith(
		v1alpha1.Condition{
			Type:               v1alpha1.ElasticsearchAutoscalerActive,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: now,
			Message:            "Autoscaler is unhealthy",
		},
		v1alpha1.Condition{
			Type:               v1alpha1.ElasticsearchAutoscalerHealthy,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: now,
			Message:            message,
		},
		v1alpha1.Condition{
			Type:               v1alpha1.ElasticsearchAutoscalerOnline,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: now,
			Message:            "Autoscaler is unhealthy",
		},
	)
	// Insert a new limited status if there is none.
	if newStatus.Conditions.Index(v1alpha1.ElasticsearchAutoscalerLimited) < 0 {
		newStatus.Conditions = newStatus.Conditions.MergeWith(v1alpha1.Condition{
			Type:               v1alpha1.ElasticsearchAutoscalerLimited,
			Status:             corev1.ConditionUnknown,
			LastTransitionTime: now,
		})
	}
	esa.Status = *newStatus
	return r.updateStatus(ctx, log, esa)
}

// reportAsInactive reports the autoscaler as inactive in the status.
func (r *ReconcileElasticsearchAutoscaler) reportAsInactive(
	ctx context.Context,
	log logr.Logger,
	esa autoscalingv1alpha1.ElasticsearchAutoscaler,
	message string,
) (reconcile.Result, error) {
	now := metav1.Now()
	newStatus := esa.Status.DeepCopy()
	newStatus.ObservedGeneration = ptr.To[int64](esa.Generation)
	newStatus.Conditions = newStatus.Conditions.MergeWith(
		v1alpha1.Condition{
			Type:               v1alpha1.ElasticsearchAutoscalerActive,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: now,
			Message:            message,
		},
		v1alpha1.Condition{
			Type:               v1alpha1.ElasticsearchAutoscalerHealthy,
			Status:             corev1.ConditionUnknown,
			LastTransitionTime: now,
			Message:            "Autoscaler is inactive",
		},
		v1alpha1.Condition{
			Type:               v1alpha1.ElasticsearchAutoscalerOnline,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: now,
			Message:            "Autoscaler is inactive",
		},
		v1alpha1.Condition{
			Type:               v1alpha1.ElasticsearchAutoscalerLimited,
			Status:             corev1.ConditionUnknown,
			LastTransitionTime: now,
		},
	)
	esa.Status = *newStatus
	return r.updateStatus(ctx, log, esa)
}

func (r *ReconcileElasticsearchAutoscaler) updateStatus(
	ctx context.Context,
	log logr.Logger,
	esa autoscalingv1alpha1.ElasticsearchAutoscaler,
) (reconcile.Result, error) {
	results := &reconciler.Results{}
	if err := r.Client.Status().Update(ctx, &esa); err != nil {
		if apierrors.IsConflict(err) {
			log.V(1).Info(
				"Conflict while updating the status",
				"namespace", esa.Namespace,
				"esa_name", esa.Name,
				"error", err.Error(),
			)
			return results.WithResult(reconcile.Result{Requeue: true}).Aggregate()
		}
		return results.WithError(tracing.CaptureError(ctx, err)).Aggregate()
	}
	return results.Aggregate()
}

func defaultResult(autoscalingSpecification v1alpha1.AutoscalingResource) *reconciler.Results {
	results := reconciler.Results{}
	requeueAfter := v1alpha1.DefaultPollingPeriod
	pollingPeriod, err := autoscalingSpecification.GetPollingPeriod()
	if err != nil {
		return results.WithError(err)
	}
	if pollingPeriod != nil {
		requeueAfter = pollingPeriod.Duration
	}
	return results.WithResult(
		reconcile.Result{
			Requeue:      true,
			RequeueAfter: requeueAfter,
		})
}
