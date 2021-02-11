// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"fmt"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/autoscaler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/resources"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/status"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	logconf "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/go-logr/logr"
	"go.elastic.co/apm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileElasticsearch) reconcileInternal(
	ctx context.Context,
	autoscalingStatus status.Status,
	autoscaledNodeSets esv1.AutoscaledNodeSets,
	autoscalingSpec esv1.AutoscalingSpec,
	es esv1.Elasticsearch,
) (reconcile.Result, error) {
	defer tracing.Span(&ctx)()
	results := &reconciler.Results{}
	log := logconf.FromContext(ctx)

	if esReachable, err := r.isElasticsearchReachable(ctx, es); !esReachable || err != nil {
		// Elasticsearch is not reachable, or we got an error while checking Elasticsearch availability, follow up with an offline reconciliation.
		if err != nil {
			log.V(1).Info(
				"error while checking if Elasticsearch is available, attempting offline reconciliation",
				"error.message", err.Error(),
			)
		}
		return r.doOfflineReconciliation(ctx, autoscalingStatus, autoscaledNodeSets, autoscalingSpec, results)
	}

	// Cluster is expected to be online and reachable, attempt a call to the autoscaling API.
	// If an error occurs we still attempt an offline reconciliation to enforce limits set by the user.
	result, err := r.attemptOnlineReconciliation(ctx, autoscalingStatus, autoscaledNodeSets, autoscalingSpec, results)
	if err != nil {
		log.Error(tracing.CaptureError(ctx, err), "autoscaling online reconciliation failed")
		// Attempt an offline reconciliation
		if _, err := r.doOfflineReconciliation(ctx, autoscalingStatus, autoscaledNodeSets, autoscalingSpec, results); err != nil {
			log.Error(tracing.CaptureError(ctx, err), "autoscaling offline reconciliation failed")
		}
	}
	return result, err
}

// Check if the Service is available.
func (r *ReconcileElasticsearch) isElasticsearchReachable(ctx context.Context, es esv1.Elasticsearch) (bool, error) {
	defer tracing.Span(&ctx)()
	externalService, err := services.GetExternalService(r.Client, es)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, tracing.CaptureError(ctx, err)
	}
	esReachable, err := services.IsServiceReady(r.Client, externalService)
	if err != nil {
		return false, tracing.CaptureError(ctx, err)
	}
	return esReachable, nil
}

// attemptOnlineReconciliation attempts an online autoscaling reconciliation with a call to the Elasticsearch autoscaling API.
func (r *ReconcileElasticsearch) attemptOnlineReconciliation(
	ctx context.Context,
	currentAutoscalingStatus status.Status,
	autoscaledNodeSets esv1.AutoscaledNodeSets,
	autoscalingSpec esv1.AutoscalingSpec,
	results *reconciler.Results,
) (reconcile.Result, error) {
	span, _ := apm.StartSpan(ctx, "online_reconciliation", tracing.SpanTypeApp)
	defer span.End()
	log := logconf.FromContext(ctx)
	log.V(1).Info("Starting online autoscaling reconciliation")
	esClient, err := r.esClientProvider(ctx, r.Client, r.Dialer, autoscalingSpec.Elasticsearch)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Update Machine Learning settings
	mlNodes, maxMemory := autoscalingSpec.GetMLNodesSettings()
	if err := esClient.UpdateMLNodesSettings(ctx, mlNodes, maxMemory); err != nil {
		log.Error(err, "Error while updating the ML settings")
		return reconcile.Result{}, err
	}

	// Update autoscaling policies in Elasticsearch
	if err := updatePolicies(ctx, log, autoscalingSpec, esClient); err != nil {
		log.Error(err, "Error while updating the autoscaling policies")
		return reconcile.Result{}, err
	}

	// Get capacity requirements from the Elasticsearch autoscaling capacity API
	requiredCapacity, err := esClient.GetAutoscalingCapacity(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Init. a new autoscaling status.
	statusBuilder := status.NewAutoscalingStatusBuilder()

	// nextClusterResources holds the resources computed by the autoscaling algorithm for each nodeSet.
	var nextClusterResources resources.ClusterResources

	// For each autoscaling policy we compute the resources to be applied to the related nodeSets.
	for _, autoscalingPolicy := range autoscalingSpec.AutoscalingPolicySpecs {
		// Get the currentNodeSets
		nodeSetList, exists := autoscaledNodeSets[autoscalingPolicy.Name]
		if !exists {
			// This situation should be caught during the validation, we still want to trace this error if it happens.
			err := fmt.Errorf("no nodeSets for tier %s", autoscalingPolicy.Name)
			log.Error(err, "no nodeSet for a tier", "policy", autoscalingPolicy.Name)
			results.WithError(fmt.Errorf("no nodeSets for tier %s", autoscalingPolicy.Name))
			statusBuilder.ForPolicy(autoscalingPolicy.Name).RecordEvent(status.NoNodeSet, err.Error())
			continue
		}

		// Get the required capacity for this autoscaling policy from the Elasticsearch API
		var nodeSetsResources resources.NodeSetsResources
		capacity, hasCapacity := requiredCapacity.Policies[autoscalingPolicy.Name]
		if hasCapacity && !capacity.RequiredCapacity.IsEmpty() {
			// We received a required capacity from Elasticsearch for this policy.
			log.Info(
				"Required capacity for policy",
				"policy", autoscalingPolicy.Name,
				"required_capacity", capacity.RequiredCapacity,
				"current_capacity", capacity.CurrentCapacity,
				"current_capacity.count", len(capacity.CurrentNodes),
				"current_nodes", capacity.CurrentNodes)
			// Ensure that the user provides the related resources policies
			if !canDecide(log, capacity.RequiredCapacity, autoscalingPolicy, statusBuilder) {
				continue
			}
			ctx := autoscaler.Context{
				Log:                      log,
				AutoscalingSpec:          autoscalingPolicy,
				NodeSets:                 nodeSetList,
				CurrentAutoscalingStatus: currentAutoscalingStatus,
				RequiredCapacity:         capacity.RequiredCapacity,
				StatusBuilder:            statusBuilder,
			}
			nodeSetsResources = ctx.GetResources()
		} else {
			// We didn't receive a required capacity for this tier, or the response is empty. We can only ensure that resources are within the allowed ranges.
			log.V(1).Info(
				"No required capacity received from Elasticsearch, ensure resources limits are respected",
				"policy", autoscalingPolicy.Name,
			)
			statusBuilder.ForPolicy(autoscalingPolicy.Name).RecordEvent(status.EmptyResponse, "No required capacity from Elasticsearch")
			nodeSetsResources = autoscaler.GetOfflineNodeSetsResources(log, nodeSetList.Names(), autoscalingPolicy, currentAutoscalingStatus)
		}
		// Add the result to the list of the next resources
		nextClusterResources = append(nextClusterResources, nodeSetsResources)
	}

	// Emit the K8S events
	status.EmitEvents(autoscalingSpec.Elasticsearch, r.recorder, statusBuilder.Build())

	// Update the Elasticsearch resource with the calculated resources.
	if err := reconcileElasticsearch(log, &autoscalingSpec.Elasticsearch, statusBuilder, nextClusterResources, currentAutoscalingStatus); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if results.HasError() {
		return results.Aggregate()
	}

	// Apply the update Elasticsearch manifest
	if err := r.Client.Update(context.Background(), &autoscalingSpec.Elasticsearch); err != nil {
		if apierrors.IsConflict(err) {
			return results.WithResult(reconcile.Result{Requeue: true}).Aggregate()
		}
		return results.WithError(err).Aggregate()
	}
	return reconcile.Result{}, nil
}

// canDecide ensures that the user has provided resource ranges to process the Elasticsearch API autoscaling response.
// Expected ranges are not consistent across all deciders. For example ml may only require memory limits, while processing
// data deciders response may require storage limits.
// Only memory and storage are supported since CPU is not part of the autoscaling API specification.
func canDecide(log logr.Logger, requiredCapacity esclient.AutoscalingCapacityInfo, spec esv1.AutoscalingPolicySpec, statusBuilder *status.AutoscalingStatusBuilder) bool {
	result := true
	if (requiredCapacity.Node.Memory != nil || requiredCapacity.Total.Memory != nil) && !spec.IsMemoryDefined() {
		log.Error(fmt.Errorf("min and max memory must be specified"), "Min and max memory must be specified", "policy", spec.Name)
		statusBuilder.ForPolicy(spec.Name).RecordEvent(status.MemoryRequired, "Min and max memory must be specified")
		result = false
	}
	if (requiredCapacity.Node.Storage != nil || requiredCapacity.Total.Storage != nil) && !spec.IsStorageDefined() {
		log.Error(fmt.Errorf("min and max memory must be specified"), "Min and max storage must be specified", "policy", spec.Name)
		statusBuilder.ForPolicy(spec.Name).RecordEvent(status.StorageRequired, "Min and max storage must be specified")
		result = false
	}
	return result
}

// doOfflineReconciliation runs an autoscaling reconciliation if the autoscaling API is not ready (yet).
func (r *ReconcileElasticsearch) doOfflineReconciliation(
	ctx context.Context,
	currentAutoscalingStatus status.Status,
	autoscaledNodeSets esv1.AutoscaledNodeSets,
	autoscalingSpec esv1.AutoscalingSpec,
	results *reconciler.Results,
) (reconcile.Result, error) {
	defer tracing.Span(&ctx)()
	log := logconf.FromContext(ctx)
	log.V(1).Info("Starting offline autoscaling reconciliation")
	statusBuilder := status.NewAutoscalingStatusBuilder()
	var clusterNodeSetsResources resources.ClusterResources
	// Elasticsearch is not reachable, we still want to ensure that min. requirements are set
	for _, autoscalingSpec := range autoscalingSpec.AutoscalingPolicySpecs {
		nodeSets, exists := autoscaledNodeSets[autoscalingSpec.Name]
		if !exists {
			return results.WithError(fmt.Errorf("no nodeSets for tier %s", autoscalingSpec.Name)).Aggregate()
		}
		nodeSetsResources := autoscaler.GetOfflineNodeSetsResources(log, nodeSets.Names(), autoscalingSpec, currentAutoscalingStatus)
		clusterNodeSetsResources = append(clusterNodeSetsResources, nodeSetsResources)
	}

	// Emit the K8S events
	status.EmitEvents(autoscalingSpec.Elasticsearch, r.recorder, statusBuilder.Build())

	// Update the Elasticsearch manifest
	if err := reconcileElasticsearch(log, &autoscalingSpec.Elasticsearch, statusBuilder, clusterNodeSetsResources, currentAutoscalingStatus); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// Apply the updated Elasticsearch manifest
	if err := r.Client.Update(context.Background(), &autoscalingSpec.Elasticsearch); err != nil {
		if apierrors.IsConflict(err) {
			return results.WithResult(reconcile.Result{Requeue: true}).Aggregate()
		}
		return results.WithError(err).Aggregate()
	}
	return results.WithResult(defaultRequeue).Aggregate()
}
