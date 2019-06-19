// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package discovery

import (
	"context"
	"strconv"
	"sync"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	common "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"
	espod "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	esversion "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("discovery")

	Zen2MinimumVersion = version.MustParse("7.0.0")
)

// ApplyZen1Limitations applies limitations on concurrent cluster changes when changing Zen1 nodes.
//
// This is done in order to avoid split-brain scenarios when creating several Zen1 nodes at once and to reduce the risk
// of reducing the minimum master nodes setting just before deleting Zen1 nodes.
//
// This method performs its own version checks and is safe to call when there's Zen2 nodes in the cluster.
func ApplyZen1Limitations(
	c k8s.Client,
	podsState mutation.PodsState,
	performableChanges *mutation.PerformableChanges,
	isElasticsearchReachable bool,
) error {
	// TODO: special-case bootstrapping?

	changingPods := append(performableChanges.ToCreate.Pods(), performableChanges.ToDelete.Pods()...)
	if len(changingPods) == 0 {
		// no changes, nothing to do
		return nil
	}

	minVersion, err := esversion.MinVersion(changingPods)
	if err != nil {
		return err
	}

	if minVersion.IsSameOrAfter(Zen2MinimumVersion) {
		// all changing nodes are zen2, no limitations to changes.
		return nil
	}

	var masterEligiblePods []v1.Pod
	availableMasterPods := 0
	for _, pod := range podsState.AllPods() {
		if label.IsMasterNode(pod) {
			masterEligiblePods = append(masterEligiblePods, pod)

			if _, ok := podsState.RunningReady[pod.Name]; ok {
				availableMasterPods += 1
			}
		}
	}

	createMasterLimit, err := calculateCreateMasterLimit(c, masterEligiblePods, availableMasterPods, isElasticsearchReachable)
	if err != nil {
		return err
	}

	// filter .ToCreate se we only create master nodes up to the limit.
	var newToCreate mutation.PodsToCreate
	creatingMasterPodsCounter := 0
	for _, podToCreate := range performableChanges.ToCreate {
		if label.IsMasterNode(podToCreate.Pod) {
			if creatingMasterPodsCounter < createMasterLimit {
				newToCreate = append(newToCreate, podToCreate)
				creatingMasterPodsCounter++
			}
		} else {
			newToCreate = append(newToCreate, podToCreate)
		}
	}

	deleteMasterLimit := calculateDeleteMasterLimit(len(masterEligiblePods), creatingMasterPodsCounter)

	var newToDelete espod.PodsWithConfig
	deletingMasterPodsCounter := 0
	for _, podToDelete := range performableChanges.ToDelete {
		if label.IsMasterNode(podToDelete.Pod) {
			if deletingMasterPodsCounter < deleteMasterLimit {
				newToDelete = append(newToDelete, podToDelete)
				deletingMasterPodsCounter++
			}
		} else {
			newToDelete = append(newToDelete, podToDelete)
		}
	}
	performableChanges.ToDelete = newToDelete
	performableChanges.ToCreate = newToCreate

	log.Info(
		"Limiting number of concurrent master changes.",
		"create_limit", createMasterLimit,
		"delete_limit", deleteMasterLimit,
		"master_eligible", masterEligiblePods,
		"master_available", availableMasterPods,
	)

	return nil
}

func calculateCreateMasterLimit(
	client k8s.Client,
	masterEligiblePods []v1.Pod,
	availableMasterPods int,
	isElasticsearchReachable bool,
) (int, error) {
	masterEligiblePodsCount := len(masterEligiblePods)

	// by default, it should be safe to create 1 master
	createMasterLimit := 1
	quorumSize := quorum(masterEligiblePodsCount + createMasterLimit)

	// but if one or more master eligible nodes are not currently part of the cluster, OR the cluster is not reachable:
	if masterEligiblePodsCount != availableMasterPods || !isElasticsearchReachable {
		createMasterLimit = 0
		quorumSize := quorum(masterEligiblePodsCount)

		// use the current lowest quorum size from the master eligible pods to avoid a split brain.
		for _, pod := range masterEligiblePods {
			cfg, err := settings.GetESConfigContent(client, k8s.ExtractNamespacedName(&pod))
			if err != nil {
				return 0, err
			}
			podSettings, err := cfg.Unpack()
			if err != nil {
				return 0, err
			}
			podQuorumSize := podSettings.Discovery.Zen.MinimumMasterNodes
			if podQuorumSize < quorumSize {
				quorumSize = podQuorumSize
			}
		}
	}

	// it's ok to add masters for as long as the quorum size for the available pods does not change
	for quorumSize == quorum(masterEligiblePodsCount+createMasterLimit+1) {
		createMasterLimit++
	}

	// special case: if we only have 1 node in a cluster, we can only add one
	if masterEligiblePodsCount == 1 && createMasterLimit > 1 {
		createMasterLimit = 1
	}

	return createMasterLimit, nil
}

func calculateDeleteMasterLimit(masterEligiblePods int, creatingMasterPods int) int {
	// by default it should be safe to delete 1 master
	deleteMasterLimit := 1
	// if we create master nodes, do not delete master nodes in the same iteration.
	if creatingMasterPods > 0 {
		deleteMasterLimit = 0
	} else {
		// if we do not create master nodes, we can delete masters until we reach the new quorum size
		quorumSize := quorum(masterEligiblePods)

		for quorumSize <= masterEligiblePods-(deleteMasterLimit+1) {
			deleteMasterLimit++
		}
	}
	return deleteMasterLimit
}

// Zen1UpdateMinimumMasterNodesClusterSettings attempts to update the minimum master nodes Elasticsearch cluster
// setting through the API.
func Zen1UpdateMinimumMasterNodesClusterSettings(
	esClient client.Client,
	podsState mutation.PodsState,
	performableChanges *mutation.PerformableChanges,
	reconcileState *reconcile.State,
) error {
	minimumMasterNodes := quorum(masterNodesWithChanges(len(podsState.MasterEligiblePods()), performableChanges))

	// Check if we really need to update minimum_master_nodes with an API call
	if minimumMasterNodes == reconcileState.GetZen1MinimumMasterNodes() {
		return nil
	}

	log.Info(
		"Update minimum master nodes",
		"how", "api",
		"minimum_master_nodes", minimumMasterNodes,
	)
	ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
	defer cancel()
	if err := esClient.SetMinimumMasterNodes(ctx, minimumMasterNodes); err != nil {
		return err
	}
	// Save the current value in the status
	reconcileState.UpdateZen1MinimumMasterNodes(minimumMasterNodes)
	return nil
}

// Zen1UpdateMinimumMasterNodesConfig updates the secrets that contains the configuration of the nodes with the
// expected value of discovery.zen.minimum_master_nodes for both the current pods and the changes that are about to be
// applied.
//
// This method performs its own version checks and is safe to call when there's Zen2 nodes in the cluster.
func Zen1UpdateMinimumMasterNodesConfig(
	c k8s.Client,
	es v1alpha1.Elasticsearch,
	podsState mutation.PodsState,
	performableChanges *mutation.PerformableChanges,
) error {
	minimumMasterNodes := quorum(masterNodesWithChanges(len(podsState.MasterEligiblePods()), performableChanges))

	// only log the zen1-related info line if there's config to update, and only once per reconciliation iteration
	once := sync.Once{}
	emitLog := func() {
		log.Info(
			"Updating Zen1-related configuration files",
			"minimum_master_nodes", minimumMasterNodes,
			"pods", podsState.Summary(),
		)
	}

	for _, p := range podsState.AllPods() {
		if isZen1MasterNode, err := podIsMasterNodeUsingZen1(p); err != nil {
			return err
		} else if !isZen1MasterNode {
			continue
		}
		once.Do(emitLog)

		config, err := settings.GetESConfigContent(c, k8s.ExtractNamespacedName(&p))
		if err != nil {
			return err
		}

		if err := config.MergeWith(
			common.MustNewSingleValue(
				settings.DiscoveryZenMinimumMasterNodes,
				strconv.Itoa(minimumMasterNodes),
			),
		); err != nil {
			return err
		}
		if err := settings.ReconcileConfig(c, es, p, config); err != nil {
			return err
		}
	}

	// Update the current value for each new podToDelete that is about to be created
	for _, change := range performableChanges.ToCreate {
		if isZen1MasterNode, err := podIsMasterNodeUsingZen1(change.Pod); err != nil {
			return err
		} else if !isZen1MasterNode {
			continue
		}
		once.Do(emitLog)

		// Update the minimum_master_nodes before podToDelete creation in order to avoid split brain situation.
		if err := change.PodSpecCtx.Config.MergeWith(
			common.MustNewSingleValue(
				settings.DiscoveryZenMinimumMasterNodes,
				strconv.Itoa(minimumMasterNodes),
			),
		); err != nil {
			return err
		}
	}

	return nil
}

func masterNodesWithChanges(masterNodes int, performableChanges *mutation.PerformableChanges) int {
	for _, pod := range performableChanges.ToCreate.Pods() {
		if label.IsMasterNode(pod) {
			masterNodes++
		}
	}
	for _, pod := range performableChanges.ToDelete.Pods() {
		if label.IsMasterNode(pod) {
			masterNodes--
		}
	}
	return masterNodes
}

// quorum computes the quorum of a cluster given the number of masters.
func quorum(nMasters int) int {
	if nMasters == 0 {
		return 0
	}
	return (nMasters / 2) + 1
}

func podIsMasterNodeUsingZen1(pod v1.Pod) (bool, error) {
	if !label.IsMasterNode(pod) {
		return false, nil
	}
	if v, err := label.ExtractVersion(pod); err != nil {
		return false, err
	} else if v != nil && v.IsSameOrAfter(Zen2MinimumVersion) {
		return false, nil
	}

	return true, nil
}
