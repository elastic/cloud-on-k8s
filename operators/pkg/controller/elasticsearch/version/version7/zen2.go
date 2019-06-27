// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version7

import (
	"context"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("version7")
)

func UpdateZen2Settings(
	esClient esclient.Client,
	minVersion version.Version,
	performableChanges mutation.PerformableChanges,
	podsState mutation.PodsState,
) error {
	// only applies to ES v7+
	if !minVersion.IsSameOrAfter(version.MustParse("7.0.0")) {
		log.Info("not setting zen2 exclusions", "min version in cluster", minVersion)
		return nil
	}

	// Voting config exclusions allow master nodes to be excluded from voting when they are
	// going to be removed from the cluster.
	// This is necessary for cases where more than half of the master nodes are deleted "too quickly"
	// (the actual time window being hard to determine).
	// For safety, we add each master node we delete to that list, prior to deletion.
	// Once the deletion is over, we need to remove corresponding nodes from that list.
	// This is particularly important during rolling upgrades with PVC reuse: we want to make sure
	// the replacing pod is allowed to vote.

	// If there are no pods currently being deleted, clear the list of voting exclusions.
	// Our cache of nodes being deleted is up-to-date thanks to pods expectations.
	mastersDeletionInProgress := false
	for _, p := range podsState.Deleting {
		if label.IsMasterNode(p) {
			mastersDeletionInProgress = true
		}
	}
	if !mastersDeletionInProgress {
		log.Info("Ensuring no voting exclusions are set")
		ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
		defer cancel()
		// TODO: optimize by doing the call only if necessary.  /!\ must make sure to clear if should be cleared
		//  during a rolling upgrade. Working with a stale cache showing a (wrong) empty list, leading us to
		//  skip the call, would be dangerous.
		if err := esClient.DeleteVotingConfigExclusions(ctx, false); err != nil {
			return err
		}
	} else {
		log.V(1).Info("Waiting for pods deletion to be over before updating voting exclusions")
	}

	// Exclude master nodes to delete from voting.
	leavingMasters := make([]string, 0, len(performableChanges.ToDelete))
	for _, p := range performableChanges.ToDelete {
		if label.IsMasterNode(p.Pod) {
			leavingMasters = append(leavingMasters, p.Pod.Name)
		}
	}
	if len(leavingMasters) > 0 {
		log.Info("Setting voting config exclusions", "excluding", leavingMasters)
		ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
		defer cancel()
		// TODO: optimize by doing the call only if necessary. /!\ must make sure to add if should be added.
		//  Working with a stale cache showing (wrongly) the node already in the list, leading us to skip the call,
		//  would be dangerous.
		if err := esClient.AddVotingConfigExclusions(ctx, leavingMasters, ""); err != nil {
			return err
		}
	}

	return nil
}
