// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"fmt"
	"sort"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/go-logr/logr"
	"go.elastic.co/apm/v2"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/autoscaling/elasticsearch/autoscaler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/autoscaling/elasticsearch/status"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/services"
	logconf "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

func (r *baseReconcileAutoscaling) reconcileInternal(
	ctx context.Context,
	es esv1.Elasticsearch,
	statusBuilder *v1alpha1.AutoscalingStatusBuilder,
	autoscaledNodeSets esv1.AutoscaledNodeSets,
	autoscalingResource v1alpha1.AutoscalingResource,
) (*esv1.Elasticsearch, error) {
	defer tracing.Span(&ctx)()
	log := logconf.FromContext(ctx)
	if !r.isElasticsearchReachable(ctx, es) {
		// Elasticsearch is not reachable, or we got an error while checking Elasticsearch availability, follow up with an offline reconciliation.
		statusBuilder.SetOnline(false, "Elasticsearch is not available")
		return r.doOfflineReconciliation(ctx, es, statusBuilder, autoscaledNodeSets, autoscalingResource)
	}
	statusBuilder.SetOnline(true, "Elasticsearch is available")
	// Cluster is expected to be online and reachable, attempt a call to the autoscaling API.
	// If an error occurs we still attempt an offline reconciliation to enforce limits set by the user.
	reconciledEs, err := r.attemptOnlineReconciliation(ctx, es, statusBuilder, autoscaledNodeSets, autoscalingResource)
	if err != nil {
		statusBuilder.SetOnline(false, err.Error())
		log.Error(tracing.CaptureError(ctx, err), "autoscaling online reconciliation failed")
		// Attempt an offline reconciliation
		offlineReconciledEs, err := r.doOfflineReconciliation(ctx, es, statusBuilder, autoscaledNodeSets, autoscalingResource)
		if err != nil {
			log.Error(tracing.CaptureError(ctx, err), "autoscaling offline reconciliation failed")
		}
		// Offline reconciliation might have returned an updated Elasticsearch resource, even if there is an error, as a best effort.
		reconciledEs = offlineReconciledEs
	}
	return reconciledEs, err
}

// newStatusBuilder creates a new status builder and initializes it with overlapping policies.
func newStatusBuilder(log logr.Logger, autoscalingPolicies v1alpha1.AutoscalingPolicySpecs) *v1alpha1.AutoscalingStatusBuilder {
	statusBuilder := v1alpha1.NewAutoscalingStatusBuilder()
	policiesByRole := autoscalingPolicies.AutoscalingPoliciesByRole()

	// Roles are sorted for consistent comparison
	roles := make([]string, 0, len(policiesByRole))
	for k := range policiesByRole {
		roles = append(roles, k)
	}
	sort.Strings(roles)

	for _, role := range roles {
		if role == string(esv1.RemoteClusterClientRole) {
			continue
		}
		policies := policiesByRole[role]
		if len(policies) < 2 {
			// This role is declared in only one autoscaling policy
			continue
		}
		// sort policies for consistent comparison
		sort.Strings(policies)
		message := fmt.Sprintf("role %s is declared in autoscaling policies %s", role, strings.Join(policies, ","))
		for _, policy := range policies {
			log.Info(message, "policy", policy)
			statusBuilder.ForPolicy(policy).RecordEvent(v1alpha1.OverlappingPolicies, message)
		}
	}
	return statusBuilder
}

// Check if the Service is available.
func (r *baseReconcileAutoscaling) isElasticsearchReachable(ctx context.Context, es esv1.Elasticsearch) bool {
	defer tracing.Span(&ctx)()
	return services.NewElasticsearchURLProvider(es, r.Client).HasEndpoints()
}

// attemptOnlineReconciliation attempts an online autoscaling reconciliation with a call to the Elasticsearch autoscaling API.
func (r *baseReconcileAutoscaling) attemptOnlineReconciliation(
	ctx context.Context,
	es esv1.Elasticsearch,
	statusBuilder *v1alpha1.AutoscalingStatusBuilder,
	autoscaledNodeSets esv1.AutoscaledNodeSets,
	autoscalingResource v1alpha1.AutoscalingResource,
) (*esv1.Elasticsearch, error) {
	span, ctx := apm.StartSpan(ctx, "online_reconciliation", tracing.SpanTypeApp)
	defer span.End()
	autoscalingSpec, err := autoscalingResource.GetAutoscalingPolicySpecs()
	if err != nil {
		return nil, err
	}
	log := logconf.FromContext(ctx)
	log.V(1).Info("Starting online autoscaling reconciliation")
	esClient, err := r.esClientProvider(ctx, r.Client, r.Dialer, es)
	if err != nil {
		return nil, err
	}

	// Update Machine Learning settings
	mlNodes, maxMemory := esv1.GetMLNodesSettings(autoscalingSpec)
	if err := esClient.UpdateMLNodesSettings(ctx, mlNodes, maxMemory); err != nil {
		log.Error(err, "Error while updating the ML settings")
		return nil, err
	}

	// Update autoscaling policies in Elasticsearch
	if err := updatePolicies(ctx, log, autoscalingResource, esClient); err != nil {
		log.Error(err, "Error while updating the autoscaling policies")
		return nil, err
	}

	// Get capacity requirements from the Elasticsearch autoscaling capacity API
	autoscalingCapacityResult, err := esClient.GetAutoscalingCapacity(ctx)
	if err != nil {
		return nil, err
	}

	// nextClusterResources holds the resources computed by the autoscaling algorithm for each nodeSet.
	var nextClusterResources v1alpha1.ClusterResources

	currentAutoscalingStatus, err := autoscalingResource.GetElasticsearchAutoscalerStatus()
	if err != nil {
		return nil, err
	}
	var errors []error
	// For each autoscaling policy we compute the resources to be applied to the related nodeSets.
	for _, autoscalingPolicy := range autoscalingSpec {
		// Get the currentNodeSets
		nodeSetList, exists := autoscaledNodeSets[autoscalingPolicy.Name]
		if !exists {
			// This situation should be caught during the validation, we still want to trace this error if it happens.
			err := fmt.Errorf("no nodeSets for tier %s", autoscalingPolicy.Name)
			log.Error(err, "no nodeSet for a tier", "policy", autoscalingPolicy.Name)
			errors = append(errors, err)
			statusBuilder.ForPolicy(autoscalingPolicy.Name).RecordEvent(v1alpha1.NoNodeSet, err.Error())
			continue
		}

		// Get the required capacity for this autoscaling policy from the Elasticsearch API
		var nodeSetsResources v1alpha1.NodeSetsResources
		autoscalingPolicyResult, hasCapacity := autoscalingCapacityResult.Policies[autoscalingPolicy.Name]
		if hasCapacity && !autoscalingPolicyResult.RequiredCapacity.IsEmpty() {
			// We received a required capacity from Elasticsearch for this policy.
			log.Info(
				"Required capacity for policy",
				"policy", autoscalingPolicy.Name,
				"required_capacity", autoscalingPolicyResult.RequiredCapacity,
				"current_capacity", autoscalingPolicyResult.CurrentCapacity,
				"current_capacity.count", len(autoscalingPolicyResult.CurrentNodes),
				"current_nodes", autoscalingPolicyResult.CurrentNodes,
			)
			ctx, err := autoscaler.NewContext(
				log,
				autoscalingPolicy,
				nodeSetList,
				currentAutoscalingStatus,
				autoscalingPolicyResult,
				statusBuilder,
			)
			if err != nil {
				log.Error(err, "Error while creating autoscaling context for policy", "policy", autoscalingPolicy.Name)
				continue
			}
			nodeSetsResources = ctx.GetResources()
		} else {
			// We didn't receive a required capacity for this tier, or the response is empty. We can only ensure that resources are within the allowed ranges.
			log.V(1).Info(
				"No required capacity received from Elasticsearch, ensure resources limits are respected",
				"policy", autoscalingPolicy.Name,
			)
			statusBuilder.ForPolicy(autoscalingPolicy.Name).RecordEvent(v1alpha1.EmptyResponse, "No required capacity from Elasticsearch")
			nodeSetsResources = autoscaler.GetOfflineNodeSetsResources(log, nodeSetList.Names(), autoscalingPolicy, currentAutoscalingStatus)
		}
		// Add the result to the list of the next resources
		nextClusterResources = append(nextClusterResources, nodeSetsResources)
	}

	// Emit the K8S events
	status.EmitEvents(es, r.recorder, statusBuilder.Build())

	// Update the Elasticsearch resource with the calculated resources.
	if err := reconcileElasticsearch(log, &es, nextClusterResources); err != nil {
		errors = append(errors, err)
	}
	if len(errors) > 0 {
		return nil, tracing.CaptureError(ctx, k8serrors.NewAggregate(errors))
	}

	// Register new resources in the status
	statusBuilder.UpdateResources(nextClusterResources, currentAutoscalingStatus)

	return &es, nil
}

// doOfflineReconciliation runs an autoscaling reconciliation if the autoscaling API is not ready (yet).
func (r *baseReconcileAutoscaling) doOfflineReconciliation(
	ctx context.Context,
	es esv1.Elasticsearch,
	statusBuilder *v1alpha1.AutoscalingStatusBuilder,
	autoscaledNodeSets esv1.AutoscaledNodeSets,
	autoscalingResource v1alpha1.AutoscalingResource,
) (*esv1.Elasticsearch, error) {
	defer tracing.Span(&ctx)()
	log := logconf.FromContext(ctx)
	log.V(1).Info("Starting offline autoscaling reconciliation")
	autoscalingSpec, err := autoscalingResource.GetAutoscalingPolicySpecs()
	if err != nil {
		return nil, err
	}
	currentAutoscalingStatus, err := autoscalingResource.GetElasticsearchAutoscalerStatus()
	if err != nil {
		return nil, err
	}
	var clusterNodeSetsResources v1alpha1.ClusterResources
	// Elasticsearch is not reachable, we still want to ensure that min. requirements are set
	for _, autoscalingSpec := range autoscalingSpec {
		nodeSets, exists := autoscaledNodeSets[autoscalingSpec.Name]
		if !exists {
			return nil, tracing.CaptureError(ctx, fmt.Errorf("no nodeSets for tier %s", autoscalingSpec.Name))
		}
		nodeSetsResources := autoscaler.GetOfflineNodeSetsResources(log, nodeSets.Names(), autoscalingSpec, currentAutoscalingStatus)
		clusterNodeSetsResources = append(clusterNodeSetsResources, nodeSetsResources)
	}

	// Emit the K8S events
	status.EmitEvents(es, r.recorder, statusBuilder.Build())

	// Update the Elasticsearch manifest
	if err := reconcileElasticsearch(log, &es, clusterNodeSetsResources); err != nil {
		return nil, tracing.CaptureError(ctx, err)
	}

	// Register new resources in the status
	statusBuilder.UpdateResources(clusterNodeSetsResources, currentAutoscalingStatus)

	return &es, nil
}
