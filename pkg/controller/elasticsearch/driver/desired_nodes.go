// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"errors"
	"fmt"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/hints"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/nodespec"
)

func (d *defaultDriver) updateDesiredNodes(
	ctx context.Context,
	esClient esclient.Client,
	esReachable bool,
	expectedResources nodespec.ResourcesList,
) *reconciler.Results {
	span, ctx := apm.StartSpan(ctx, "update_desired_nodes", tracing.SpanTypeApp)
	defer span.End()
	results := &reconciler.Results{}
	// We compute the desired nodes state to update the condition
	var resourceNotAvailableErr *nodespec.ResourceNotAvailable
	esVersion, err := version.Parse(d.ES.Spec.Version)
	if err != nil {
		return results.WithError(err)
	}
	nodes, requeue, err := expectedResources.ToDesiredNodes(ctx, d.Client, esVersion.FinalizeVersion())
	switch {
	case err == nil:
		d.ReconcileState.ReportCondition(
			esv1.ResourcesAwareManagement,
			corev1.ConditionTrue,
			fmt.Sprintf("Successfully calculated compute and storage resources from Elasticsearch resource generation %d", d.ES.Generation),
		)
	case errors.As(err, &resourceNotAvailableErr):
		// It is not possible to build the desired node spec because of the Elasticsearch specification
		d.ReconcileState.ReportCondition(
			esv1.ResourcesAwareManagement,
			corev1.ConditionFalse,
			fmt.Sprintf("Cannot get compute and storage resources from Elasticsearch resource generation %d: %s", d.ES.Generation, err.Error()),
		)
		// It is fine to continue, error is only reported through the condition.
		// We should however clear the desired nodes API since we are in a degraded (not resources aware) mode.
		if esReachable {
			return results.WithError(esClient.DeleteDesiredNodes(ctx))
		}
		return results.WithReconciliationState(defaultRequeue.WithReason("Desired nodes API must be cleared"))
	default:
		// Unknown error: not nil and not ResourceNotAvailable
		d.ReconcileState.ReportCondition(
			esv1.ResourcesAwareManagement,
			corev1.ConditionUnknown,
			fmt.Sprintf("Error while calculating compute and storage resources from Elasticsearch resource generation %d: %s", d.ES.Generation, err.Error()),
		)
		return results.WithError(err)
	}
	if requeue {
		results.WithReconciliationState(defaultRequeue.WithReason("Storage capacity is not available in all PVC statuses, requeue to refine the capacity reported in the desired nodes API"))
	}
	if esReachable {
		latestDesiredNodes, err := esClient.GetLatestDesiredNodes(ctx)
		if err != nil && !esclient.IsNotFound(err) {
			// ignore 404 but error out on anything else
			return results.WithError(err)
		}

		nodesHash := hash.HashObject(nodes)
		if d.ReconcileState.OrchestrationHints().DesiredNodes.Equals(latestDesiredNodes.Version, nodesHash) {
			return results
		}

		nextVersion := latestDesiredNodes.Version + 1
		err = esClient.UpdateDesiredNodes(ctx, string(d.ES.UID), nextVersion, esclient.DesiredNodes{DesiredNodes: nodes})
		if err == nil {
			d.ReconcileState.UpdateOrchestrationHints(hints.OrchestrationsHints{DesiredNodes: &hints.DesiredNodesHint{
				Version: nextVersion,
				Hash:    nodesHash,
			}})
		}
		return results.WithError(err)
	}
	return results.WithReconciliationState(defaultRequeue.WithReason("Waiting for Elasticsearch to be available to update the desired nodes API"))
}
