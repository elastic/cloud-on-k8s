// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen1

import (
	"context"
	"strconv"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	common "github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("zen1")
)

// SetupMinimumMasterNodesConfig modifies the ES config of the given resources to setup
// zen1 minimum master nodes.
// This is function should not be called unless all the expectations are met.
func SetupMinimumMasterNodesConfig(
	c k8s.Client,
	es v1alpha1.Elasticsearch,
	nodeSpecResources nodespec.ResourcesList,
) error {
	// Check if we have at least one Zen1 compatible pod or StatefulSet in flight.
	if zen1compatible, err := AtLeastOneNodeCompatibleWithZen1(nodeSpecResources.StatefulSets(), c, es); !zen1compatible || err != nil {
		return err
	}

	masters := nodeSpecResources.MasterNodesNames()
	quorum := settings.Quorum(len(masters))

	for i := range nodeSpecResources {
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

// UpdateMinimumMasterNodes calls the ES API to update the minimum_master_nodes setting if required,
// based on nodes currently running in the cluster.
// It returns true if this should be retried later (re-queued).
func UpdateMinimumMasterNodes(
	c k8s.Client,
	es v1alpha1.Elasticsearch,
	esClient client.Client,
	actualStatefulSets sset.StatefulSetList,
	reconcileState *reconcile.State,
) (bool, error) {
	// Check if we have at least one Zen1 compatible pod or StatefulSet in flight.
	if zen1compatible, err := AtLeastOneNodeCompatibleWithZen1(actualStatefulSets, c, es); !zen1compatible || err != nil {
		return false, err
	}

	actualMasters, err := sset.GetActualMastersForCluster(c, es)
	if err != nil {
		return false, err
	}

	// Get current master nodes count
	currentMasterCount := len(actualMasters)
	currentAvailableMasterCount := 0
	for _, p := range actualMasters {
		if k8s.IsPodReady(p) {
			currentAvailableMasterCount++
		}
	}
	// Calculate minimum_master_nodes based on that.
	minimumMasterNodes := settings.Quorum(currentMasterCount)

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

	return false, UpdateMinimumMasterNodesTo(es, esClient, reconcileState, minimumMasterNodes)
}

// UpdateMinimumMasterNodesTo calls the ES API to update the value of zen1 minimum_master_nodes
// to the given value, if the cluster is using zen1.
// Should only be called it there are some Zen1 compatible masters
func UpdateMinimumMasterNodesTo(
	es v1alpha1.Elasticsearch,
	esClient client.Client,
	reconcileState *reconcile.State,
	minimumMasterNodes int,
) error {
	// Check if we really need to update minimum_master_nodes with an API call
	if minimumMasterNodes == reconcileState.GetZen1MinimumMasterNodes() {
		return nil
	}

	log.Info("Updating minimum master nodes",
		"how", "api",
		"namespace", es.Namespace,
		"es_name", es.Name,
		"minimum_master_nodes", minimumMasterNodes,
	)
	ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
	defer cancel()
	if err := esClient.SetMinimumMasterNodes(ctx, minimumMasterNodes); err != nil {
		return nil
	}
	// Save the current value in the status
	reconcileState.UpdateZen1MinimumMasterNodes(minimumMasterNodes)
	return nil
}
