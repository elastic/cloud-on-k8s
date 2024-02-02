// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package zen1

import (
	"context"
	"strconv"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	common "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/settings"
	es_sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

// SetupMinimumMasterNodesConfig modifies the ES config of the given resources to setup
// zen1 minimum master nodes.
// This function should not be called unless all the expectations are met.
func SetupMinimumMasterNodesConfig(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	nodeSpecResources nodespec.ResourcesList,
) error {
	// Check if we have at least one Zen1 compatible pod or StatefulSet in flight.
	if zen1compatible, err := AtLeastOneNodeCompatibleWithZen1(
		ctx,
		nodeSpecResources.StatefulSets(), c, es,
	); !zen1compatible || err != nil {
		return err
	}

	// There are 2 possible situations here:
	// 1. The StatefulSet contains some masters: use the replicas to set m_m_n in the configuration file.
	// 2. The StatefulSet does not contain any master but there are some existing Pods: we should NOT rely on the spec
	//    of the StatefulSet since it might not reflect the situation, the node type "master" might just have been changed
	//    and a rolling upgrade is maybe in progress.
	//    In this case some masters are maybe still alive, decreasing m_m_n in the config could lead to a split brain
	//    situation if the container (not the Pod) restarts.
	masters := 0
	for _, resource := range nodeSpecResources {
		resource := resource
		if label.IsMasterNodeSet(resource.StatefulSet) {
			// First situation: just check for the replicas
			masters += int(sset.GetReplicas(resource.StatefulSet))
		} else {
			// Second situation: not a sset of masters, but we check if there are some of them waiting for a rolling upgrade
			actualPods, err := es_sset.GetActualPodsForStatefulSet(c, k8s.ExtractNamespacedName(&resource.StatefulSet))
			if err != nil {
				return err
			}
			actualMasters := len(label.FilterMasterNodePods(actualPods))
			masters += actualMasters
		}
	}

	quorum := settings.Quorum(masters)

	for i := range nodeSpecResources {
		// patch config with the expected minimum master nodes
		if err := nodeSpecResources[i].Config.MergeWith(
			common.MustNewSingleValue(
				esv1.DiscoveryZenMinimumMasterNodes,
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
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	esClient client.Client,
	actualStatefulSets es_sset.StatefulSetList,
) (bool, error) {
	// Check if we have at least one Zen1 compatible pod or StatefulSet in flight.
	if zen1compatible, err := AtLeastOneNodeCompatibleWithZen1(ctx, actualStatefulSets, c, es); !zen1compatible || err != nil {
		return false, err
	}

	actualMasters, err := es_sset.GetActualMastersForCluster(c, es)
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
		ulog.FromContext(ctx).V(1).Info("Not enough masters to update the API",
			"namespace", es.Namespace,
			"es_name", es.Name,
			"current", currentAvailableMasterCount,
			"minimum_master_nodes", minimumMasterNodes)
		// We can't update the minimum master nodes right now, it is the case if a new master node is not created yet.
		// In that case we need to requeue later.
		return true, nil
	}

	return false, UpdateMinimumMasterNodesTo(ctx, es, esClient, minimumMasterNodes)
}

// UpdateMinimumMasterNodesTo calls the ES API to update the value of zen1 minimum_master_nodes
// to the given value, if the cluster is using zen1.
// Should only be called it there are some Zen1 compatible masters
func UpdateMinimumMasterNodesTo(
	ctx context.Context,
	es esv1.Elasticsearch,
	esClient client.Client,
	minimumMasterNodes int,
) error {
	ulog.FromContext(ctx).Info("Updating minimum master nodes",
		"how", "api",
		"namespace", es.Namespace,
		"es_name", es.Name,
		"minimum_master_nodes", minimumMasterNodes,
	)
	return esClient.SetMinimumMasterNodes(ctx, minimumMasterNodes)
}
