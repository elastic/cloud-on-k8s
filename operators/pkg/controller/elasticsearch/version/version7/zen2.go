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
	changes mutation.Changes,
	performableChanges mutation.PerformableChanges,
) error {
	if !minVersion.IsSameOrAfter(version.MustParse("7.0.0")) {
		log.Info("not setting zen2 exclusions", "min version in cluster", minVersion)
		return nil
	}
	if !changes.HasChanges() {
		log.Info("Ensuring no voting exclusions are set")
		ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
		defer cancel()
		if err := esClient.DeleteVotingConfigExclusions(ctx, false); err != nil {
			return err
		}
		return nil
	}

	leavingMasters := make([]string, 0)
	for _, pod := range performableChanges.ToDelete {
		if label.IsMasterNode(pod.Pod) {
			leavingMasters = append(leavingMasters, pod.Pod.Name)
		}
	}
	if len(leavingMasters) != 0 {
		// TODO: only update if required and remove old exclusions as well
		log.Info("Setting voting config exclusions", "excluding", leavingMasters)
		ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
		defer cancel()
		if err := esClient.AddVotingConfigExclusions(ctx, leavingMasters, ""); err != nil {
			return err
		}
	}
	return nil
}
