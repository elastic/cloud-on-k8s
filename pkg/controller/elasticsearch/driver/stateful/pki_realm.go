// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver/shared"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// reconcilePKIRoleMappings reconciles the ECK PKI role mappings and applies the pending conflict-free evaluations once
// the ECK-managed PKI realm runs on all the nodes and Elasticsearch applied the file-based settings holding the PKI
// role mappings: a file-based settings version Elasticsearch fails to apply, e.g. because of an invalid
// StackConfigPolicy, must not switch associations to PKI. Elasticsearch is only queried when some are pending. Until
// then it requeues, also while the orchestration is paused: nodes can join, leave or restart, and file-based settings
// be applied, without an event triggering a reconciliation.
func reconcilePKIRoleMappings(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	pkiRealmConfigured bool,
	sharedState *shared.ReconcileState,
) *reconciler.Results {
	results := reconciler.NewResult(ctx)
	evaluations, err := filesettings.ReconcilePKIRoleMappings(ctx, c, es, pkiRealmConfigured)
	if err != nil {
		return results.WithError(err)
	}
	if !evaluations.Pending() {
		return results
	}
	running, err := pkiRealmRunningOnAllNodes(ctx, sharedState.ESReachable, sharedState.ESClient, sharedState.ResourcesState)
	if err != nil {
		// Elasticsearch is routinely unreachable during a rolling restart.
		ulog.FromContext(ctx).V(1).Info("Could not detect if all nodes run the ECK-managed PKI realm", "err", err)
		return results.WithReconciliationState(shared.DefaultRequeue.WithReason("Waiting for Elasticsearch to report if the ECK-managed PKI realm runs on all the nodes"))
	}
	if !running {
		return results.WithReconciliationState(shared.DefaultRequeue.WithReason("Waiting for the ECK-managed PKI realm to run on all the nodes"))
	}
	current, err := esclient.GetFileSettings(ctx, sharedState.ESClient)
	if err != nil {
		ulog.FromContext(ctx).V(1).Info("Could not get the file-based settings applied by Elasticsearch", "err", err)
		return results.WithReconciliationState(shared.DefaultRequeue.WithReason("Waiting for Elasticsearch to report the applied file-based settings"))
	}
	if !evaluations.FileSettingsApplied(current) {
		reason := fmt.Sprintf("Waiting for Elasticsearch to apply the file-based settings version %d holding the PKI role mappings, current version is %d",
			evaluations.FileSettingsVersion(), current.Version)
		if current.Errors != nil && current.Errors.Version >= evaluations.FileSettingsVersion() {
			reason = fmt.Sprintf("Elasticsearch failed to apply the file-based settings version %d holding the PKI role mappings, current version is %d",
				current.Errors.Version, current.Version)
		}
		return results.WithReconciliationState(shared.DefaultRequeue.WithReason(reason))
	}
	return results.WithError(evaluations.Apply(ctx, c))
}

// pkiRealmRunningOnAllNodes reports whether every node reports the node attribute set with the ECK-managed PKI realm
// (commonv1.ECKPKIRealmNodeAttribute). The realm is a static setting loaded on restart, and the StatefulSets are only
// updated later in the reconciliation, so their rollout state cannot replace this check.
// Every Pod must be a node, Ready or not: a Pod missing from the response may run a previous configuration, such as an
// old node temporarily disconnected during the rolling restart, which can rejoin without restarting. A restarting,
// joining or unschedulable Pod therefore delays the PKI associations, which keep their credentials meanwhile.
// It returns an error when Elasticsearch cannot be queried, reports no node, or does not report every Pod as a node
// while all the nodes it reports run the realm: whether the missing Pods run it is unknown.
func pkiRealmRunningOnAllNodes(
	ctx context.Context,
	esReachable bool,
	esClient esclient.Client,
	resourcesState *reconcile.ResourcesState,
) (bool, error) {
	if !esReachable {
		return false, errors.New("elasticsearch is not reachable")
	}
	nodes, err := esClient.GetNodes(ctx)
	if err != nil {
		return false, err
	}
	if len(nodes.Nodes) == 0 {
		return false, errors.New("elasticsearch reported no node")
	}
	nodeNames := make(sets.Set[string], len(nodes.Nodes))
	for _, node := range nodes.Nodes {
		if node.Attributes[commonv1.ECKPKIRealmNodeAttribute] != "true" {
			return false, nil
		}
		nodeNames.Insert(node.Name)
	}
	for _, pod := range resourcesState.AllPods {
		if _, isNode := nodeNames[pod.Name]; !isNode {
			return false, fmt.Errorf("pod %s is not a node of the cluster", pod.Name)
		}
	}
	return true, nil
}
