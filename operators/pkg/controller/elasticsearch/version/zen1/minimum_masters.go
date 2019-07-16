// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen1

import (
	"context"
	"strconv"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	common "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

var (
	log = logf.Log.WithName("zen1")
)

// SetupMinimumMasterNodesConfig modifies the ES config of the given resources to setup
// zen1 minimum master nodes.
func SetupMinimumMasterNodesConfig(nodeSpecResources nodespec.ResourcesList) error {
	masters := nodeSpecResources.MasterNodesNames()
	quorum := settings.Quorum(len(masters))
	for i, res := range nodeSpecResources {
		if !IsCompatibleForZen1(res.StatefulSet) {
			continue
		}
		// patch config with the expected minimum master nodes
		if err := nodeSpecResources[i].Config.MergeWith(
			common.MustNewSingleValue(
				settings.DiscoveryZenMinimumMasterNodes,
				strconv.Itoa(quorum),
			),
		); err != nil {
			return err
		}
	}
	return nil
}

// UpdateMinimumMasterNodes calls the ES API to update the minimum_master_nodes setting if required.
func UpdateMinimumMasterNodes(
	c k8s.Client,
	es v1alpha1.Elasticsearch,
	esClient client.Client,
	actualStatefulSets sset.StatefulSetList,
	reconcileState *reconcile.State,
) (bool, error) {
	if !AtLeastOneNodeCompatibleForZen1(actualStatefulSets) {
		// nothing to do
		return false, nil
	}
	pods, err := actualStatefulSets.GetActualPods(c)
	if err != nil {
		return false, err
	}
	// Get current master nodes count
	currentMasterCount := 0
	// Among them get the ones that are ready
	currentAvailableMasterCount := 0
	for _, p := range pods {
		if label.IsMasterNode(p) {
			currentMasterCount++
			if k8s.IsPodReady(p) {
				currentAvailableMasterCount++
			}
		}
	}
	minimumMasterNodes := settings.Quorum(currentMasterCount)

	// Check if we really need to update minimum_master_nodes with an API call
	if minimumMasterNodes == reconcileState.GetZen1MinimumMasterNodes() {
		return false, nil
	}

	// Do not attempt to make an API call if there is not enough available masters
	if currentAvailableMasterCount < minimumMasterNodes {
		// This is expected to happen from time to time
		log.V(1).Info("Not enough masters to update the API",
			"namespace", es.Namespace,
			"es_name", es.Name,
			"current", currentAvailableMasterCount,
			"minimum_master_nodes", minimumMasterNodes)
		// We can't update the minimum master nodes right now, it is the case if a new master node is not created yet.
		// In that case we need to requeue later.
		return true, nil
	}

	log.Info("Updating minimum master nodes",
		"how", "api",
		"namespace", es.Namespace,
		"es_name", es.Name,
		"current", currentMasterCount,
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
