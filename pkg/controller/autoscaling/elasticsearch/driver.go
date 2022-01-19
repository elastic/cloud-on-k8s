// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"go.elastic.co/apm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/autoscaler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/resources"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/status"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	logconf "github.com/elastic/cloud-on-k8s/pkg/utils/log"
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
	statusBuilder := newStatusBuilder(log, autoscalingSpec)

	if esReachable, err := r.isElasticsearchReachable(ctx, es); !esReachable || err != nil {
		// Elasticsearch is not reachable, or we got an error while checking Elasticsearch availability, follow up with an offline reconciliation.
		if err != nil {
			log.V(1).Info(
				"error while checking if Elasticsearch is available, attempting offline reconciliation",
				"error.message", err.Error(),
			)
		}
		return r.doOfflineReconciliation(ctx, statusBuilder, autoscalingStatus, autoscaledNodeSets, autoscalingSpec, results)
	}

	// Cluster is expected to be online and reachable, attempt a call to the autoscaling API.
	// If an error occurs we still attempt an offline reconciliation to enforce limits set by the user.
	result, err := r.attemptOnlineReconciliation(ctx, statusBuilder, autoscalingStatus, autoscaledNodeSets, autoscalingSpec, results)
	if err != nil {
		log.Error(tracing.CaptureError(ctx, err), "autoscaling online reconciliation failed")
		// Attempt an offline reconciliation
		if _, err := r.doOfflineReconciliation(ctx, statusBuilder, autoscalingStatus, autoscaledNodeSets, autoscalingSpec, results); err != nil {
			log.Error(tracing.CaptureError(ctx, err), "autoscaling offline reconciliation failed")
		}
	}
	return result, err
}

// newStatusBuilder creates a new status builder and initializes it with overlapping policies.
func newStatusBuilder(log logr.Logger, autoscalingSpec esv1.AutoscalingSpec) *status.AutoscalingStatusBuilder {
	statusBuilder := status.NewAutoscalingStatusBuilder()
	policiesByRole := autoscalingSpec.AutoscalingPoliciesByRole()

	// Roles are sorted for consistent comparison
	roles := make([]string, 0, len(policiesByRole))
	for k := range policiesByRole {
		roles = append(roles, k)
	}
	sort.Strings(roles)

	for _, role := range roles {
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
			statusBuilder.ForPolicy(policy).RecordEvent(status.OverlappingPolicies, message)
		}
	}
	return statusBuilder
}

// Check if the Service is available.
func (r *ReconcileElasticsearch) isElasticsearchReachable(ctx context.Context, es esv1.Elasticsearch) (bool, error) {
	defer tracing.Span(&ctx)()
	internalService, err := services.GetInternalService(r.Client, es)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, tracing.CaptureError(ctx, err)
	}
	esReachable, err := services.IsServiceReady(r.Client, internalService)
	if err != nil {
		return false, tracing.CaptureError(ctx, err)
	}
	return esReachable, nil
}

// attemptOnlineReconciliation attempts an online autoscaling reconciliation with a call to the Elasticsearch autoscaling API.
func (r *ReconcileElasticsearch) attemptOnlineReconciliation(
	ctx context.Context,
	statusBuilder *status.AutoscalingStatusBuilder,
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
	autoscalingCapacityResult, err := esClient.GetAutoscalingCapacity(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

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

// doOfflineReconciliation runs an autoscaling reconciliation if the autoscaling API is not ready (yet).
func (r *ReconcileElasticsearch) doOfflineReconciliation(
	ctx context.Context,
	statusBuilder *status.AutoscalingStatusBuilder,
	currentAutoscalingStatus status.Status,
	autoscaledNodeSets esv1.AutoscaledNodeSets,
	autoscalingSpec esv1.AutoscalingSpec,
	results *reconciler.Results,
) (reconcile.Result, error) {
	defer tracing.Span(&ctx)()
	log := logconf.FromContext(ctx)
	log.V(1).Info("Starting offline autoscaling reconciliation")

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
	return results.Aggregate()
}
