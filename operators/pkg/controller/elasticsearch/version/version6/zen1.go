// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version6

import (
	"context"
	"strconv"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	common "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("version")
)

// UpdateZen1Discovery updates the secret that contains the configuration of the nodes with the expected value
// of discovery.zen.minimum_master_nodes. It also attempts to update this specific setting for the already existing nodes
// through the API. If an update can't be done immediately (e.g. because some master nodes are not created yet) then the
// function returns true in order to notify the caller that a new reconciliation loop should be triggered to try again later.
func UpdateZen1Discovery(
	cluster v1alpha1.Elasticsearch,
	c k8s.Client,
	esClient client.Client,
	allPods []corev1.Pod,
	performableChanges *mutation.PerformableChanges,
	reconcileState *reconcile.State,
) (bool, error) {
	// Get current master nodes count
	currentMasterCount := 0
	// Among them get the ones that are ready
	currentAvailableMasterCount := 0
	for _, p := range allPods {
		if label.IsMasterNode(p) {
			currentMasterCount++
			if k8s.IsPodReady(p) {
				currentAvailableMasterCount++
			}
		}
	}

	nextMasterCount := currentMasterCount
	// Add masters that must be created by this reconciliation loop
	for _, pod := range performableChanges.ToCreate.Pods() {
		if label.IsMasterNode(pod) {
			nextMasterCount++
		}
	}

	minimumMasterNodes := settings.Quorum(nextMasterCount)
	// Update the current value in the configuration of existing pods
	log.V(1).Info("Set minimum master nodes",
		"how", "configuration",
		"currentMasterCount", currentMasterCount,
		"nextMasterCount", nextMasterCount,
		"minimum_master_nodes", minimumMasterNodes,
	)
	for _, p := range allPods {
		config, err := settings.GetESConfigContent(c, p.Namespace, p.Labels[label.StatefulSetNameLabelName])
		if err != nil {
			return false, err
		}
		err = config.MergeWith(
			common.MustNewSingleValue(
				settings.DiscoveryZenMinimumMasterNodes,
				strconv.Itoa(minimumMasterNodes),
			),
		)
		if err != nil {
			return false, err
		}
		// TODO: fix for sset
		//if err := settings.ReconcileConfig(c, cluster, p, config); err != nil {
		//	return false, err
		//}
	}

	// Update the current value for each new pod that is about to be created
	for _, change := range performableChanges.ToCreate {
		// Update the minimum_master_nodes before pod creation in order to avoid split brain situation.
		err := change.PodSpecCtx.Config.MergeWith(
			common.MustNewSingleValue(
				settings.DiscoveryZenMinimumMasterNodes,
				strconv.Itoa(minimumMasterNodes),
			),
		)
		if err != nil {
			return false, err
		}
	}

	// Check if we really need to update minimum_master_nodes with a API call
	if minimumMasterNodes == reconcileState.GetZen1MinimumMasterNodes() {
		return false, nil
	}

	// Do not attempt to make an API call if there is not enough available masters
	if currentAvailableMasterCount < minimumMasterNodes {
		log.V(1).Info("Not enough masters to update the API",
			"current", currentAvailableMasterCount,
			"required", minimumMasterNodes)
		// We can't update the minimum master nodes right now, it is the case if a new master node is not created yet.
		// In that case we need to requeue later.
		return true, nil
	}

	log.Info("Update minimum master nodes",
		"how", "api",
		"currentMasterCount", currentMasterCount,
		"nextMasterCount", nextMasterCount,
		"minimum_master_nodes", minimumMasterNodes,
	)
	ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
	defer cancel()
	if err := esClient.SetMinimumMasterNodes(ctx, minimumMasterNodes); err != nil {
		return false, err
	}
	// Save the current value in the status
	reconcileState.UpdateZen1MinimumMasterNodes(minimumMasterNodes)
	return false, nil
}
