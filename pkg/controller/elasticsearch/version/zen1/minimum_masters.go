// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen1

import (
	"context"
	"strconv"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	common "github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	log = logf.Log.WithName("zen1")
)

const (
	Zen1MiniumMasterNodesAnnotationName = "elasticsearch.k8s.elastic.co/minimum-master-nodes"
)

// SetupMinimumMasterNodesConfig modifies the ES config of the given resources to setup
// zen1 minimum master nodes.
// This function should not be called unless all the expectations are met.
func SetupMinimumMasterNodesConfig(
	c k8s.Client,
	es v1beta1.Elasticsearch,
	nodeSpecResources nodespec.ResourcesList,
) error {
	// Check if we have at least one Zen1 compatible pod or StatefulSet in flight.
	if zen1compatible, err := AtLeastOneNodeCompatibleWithZen1(
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
		if label.IsMasterNodeSet(resource.StatefulSet) {
			// First situation: just check for the replicas
			masters += int(sset.GetReplicas(resource.StatefulSet))
		} else {
			// Second situation: not a sset of masters, but we check if there are some of them waiting for a rolling upgrade
			actualPods, err := sset.GetActualPodsForStatefulSet(c, k8s.ExtractNamespacedName(&resource.StatefulSet))
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
				v1beta1.DiscoveryZenMinimumMasterNodes,
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
	es v1beta1.Elasticsearch,
	esClient client.Client,
	actualStatefulSets sset.StatefulSetList,
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

	return false, UpdateMinimumMasterNodesTo(es, c, esClient, minimumMasterNodes)
}

// UpdateMinimumMasterNodesTo calls the ES API to update the value of zen1 minimum_master_nodes
// to the given value, if the cluster is using zen1.
// Should only be called it there are some Zen1 compatible masters
func UpdateMinimumMasterNodesTo(
	es v1beta1.Elasticsearch,
	c k8s.Client,
	esClient client.Client,
	minimumMasterNodes int,
) error {
	// Check if we really need to update minimum_master_nodes with an API call
	if minimumMasterNodes == minimumMasterNodesFromAnnotation(es) {
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
	// Save the current value in an annotation
	return annotateWithMinimumMasterNodes(c, es, minimumMasterNodes)
}

func minimumMasterNodesFromAnnotation(es v1beta1.Elasticsearch) int {
	annotationStr, set := es.Annotations[Zen1MiniumMasterNodesAnnotationName]
	if !set {
		return 0
	}
	mmn, err := strconv.Atoi(annotationStr)
	if err != nil {
		// this is an optimization only, drop the error
		return 0
	}
	return mmn
}

func annotateWithMinimumMasterNodes(c k8s.Client, es v1beta1.Elasticsearch, minimumMasterNodes int) error {
	if es.Annotations == nil {
		es.Annotations = make(map[string]string)
	}
	es.Annotations[Zen1MiniumMasterNodesAnnotationName] = strconv.Itoa(minimumMasterNodes)
	return c.Update(&es)
}
