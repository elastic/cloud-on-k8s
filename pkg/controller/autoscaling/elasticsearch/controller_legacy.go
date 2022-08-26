// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"

	logconf "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/autoscaling/elasticsearch/status"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/validation"
)

const (
	// LegacyControllerName is the name of the legacy Elasticsearch autoscaling controller. It reads the autoscaling specification in an annotation,
	// which was the first way to expose the autoscaling API, before a dedicated CRD has been introduced.
	// While the constant name used starts with the "legacy" wording, the value starts with "deprecated" instead. It is to make it explicit to the user,
	// through the logs that this controller should no longer be used.
	LegacyControllerName = "deprecated-elasticsearch-autoscaling"

	deprecationMessage = "The use of the Elasticsearch autoscaling annotation is deprecated, please consider moving to the ElasticsearchAutoscaler custom resource."
)

// ReconcileElasticsearchAutoscalingAnnotation is the legacy autoscaling reconciler which reads autoscaling specification
// from an annotation on the Elasticsearch resource itself.
// Deprecated: the autoscaling annotation has been deprecated in favor of the ElasticsearchAutoscaler custom resource.
type ReconcileElasticsearchAutoscalingAnnotation struct {
	baseReconcileAutoscaling
}

// Reconcile updates the ResourceRequirements and PersistentVolumeClaim fields for each elasticsearch container in a
// NodeSet managed by an autoscaling policy. ResourceRequirements are updated according to the response of the Elasticsearch
// _autoscaling/capacity API and given the constraints provided by the user in the autoscaling specification.
func (r *ReconcileElasticsearchAutoscalingAnnotation) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.Tracer, LegacyControllerName, "es_name", request)
	defer common.LogReconciliationRun(logconf.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	// Fetch the Elasticsearch instance
	var es esv1.Elasticsearch
	if err := r.Get(ctx, request.NamespacedName, &es); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if !es.IsAutoscalingAnnotationSet() {
		return reconcile.Result{}, nil
	}

	log := logconf.FromContext(ctx)

	// Warn user that this controller is deprecated.
	log.Info(deprecationMessage, "namespace", es.Namespace, "es_name", es.Name)
	r.recorder.Event(&es, corev1.EventTypeWarning, events.EventReasonDeprecated, deprecationMessage)

	enabled, err := r.licenseChecker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !enabled {
		log.Info(enterpriseFeaturesDisabledMsg)
		r.recorder.Eventf(&es, corev1.EventTypeWarning, license.EventInvalidLicense, enterpriseFeaturesDisabledMsg)
		// We still schedule a reconciliation in case a valid license is applied later
		return licenseCheckRequeue, nil
	}

	if common.IsUnmanaged(ctx, &es) {
		log.Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", es.Namespace, "es_name", es.Name)
		return reconcile.Result{}, nil
	}

	// Get resource policies from the Elasticsearch spec
	autoscalingSpecification, err := es.GetAutoscalingSpecificationFromAnnotation()
	if err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// Validate Elasticsearch and Autoscaling spec
	if err := validation.ValidateElasticsearch(ctx, es, r.licenseChecker, r.ExposedNodeLabels); err != nil {
		log.Error(
			err,
			"Elasticsearch manifest validation failed",
			"namespace", es.Namespace,
			"es_name", es.Name,
		)
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// Build status from annotation or existing resources
	autoscalingStatus, err := esv1.ElasticsearchAutoscalerStatusFrom(es) //nolint:staticcheck
	if err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if len(autoscalingSpecification.AutoscalingPolicySpecs) == 0 && len(autoscalingStatus.AutoscalingPolicyStatuses) == 0 {
		// This cluster is not managed by the autoscaler
		return reconcile.Result{}, nil
	}

	// Get autoscaling policies and the associated node sets.
	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		return reconcile.Result{}, err
	}
	autoscaledNodeSets, nodeSetErr := es.GetAutoscaledNodeSets(v, autoscalingSpecification.AutoscalingPolicySpecs)
	if nodeSetErr != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, nodeSetErr)
	}
	log.V(1).Info("Autoscaling policies and node sets", "policies", autoscaledNodeSets.Names())

	// Import existing resources in the current Status if the cluster is managed by some autoscaling policies but
	// the status annotation does not exist.
	if err := status.ImportExistingResources(
		log,
		r.Client,
		autoscalingSpecification.AutoscalingPolicySpecs,
		es,
		autoscaledNodeSets,
		&autoscalingStatus,
	); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	statusBuilder := newStatusBuilder(log, autoscalingSpecification.AutoscalingPolicySpecs)
	results := &reconciler.Results{}

	// Call the main function
	reconciledEs, err := r.reconcileInternal(ctx, es, statusBuilder, autoscaledNodeSets, es)
	if err != nil {
		// we do not return immediately as not all errors prevent to compute a reconciled Elasticsearch resource.
		results.WithError(err)
	}
	if reconciledEs == nil {
		// no reconciled Elasticsearch has been returned, likely to be the case if a fatal error prevented
		// to calculate the new compute and storage resources.
		return results.Aggregate()
	}

	// Update the autoscaling status annotation
	esv1.UpdateAutoscalingStatus(reconciledEs, statusBuilder.Build()) //nolint:staticcheck

	// Update the Elasticsearch resource
	if err := r.Client.Update(ctx, reconciledEs); err != nil {
		if apierrors.IsConflict(err) {
			return results.WithResult(reconcile.Result{Requeue: true}).Aggregate()
		}
		return results.WithError(err).Aggregate()
	}
	return results.WithResults(defaultResult(es)).Aggregate()
}
