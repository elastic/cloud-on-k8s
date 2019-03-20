// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version7

import (
	"context"

	esclient "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/mutation"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("version7")
)

func UpdateZen2Settings(
	esClient esclient.Client,
	changes mutation.Changes,
	performableChanges mutation.PerformableChanges,
) error {
	if !changes.HasChanges() {
		log.Info("Ensuring no voting exclusions are set")
		if err := esClient.DeleteVotingConfigExclusions(context.TODO(), false); err != nil {
			return err
		}
		return nil
	}

	leavingMasters := make([]string, 0)
	for _, pod := range performableChanges.ToDelete {
		if label.IsMasterNode(pod) {
			leavingMasters = append(leavingMasters, pod.Name)
		}
	}
	if len(leavingMasters) != 0 {
		// TODO: only update if required and remove old exclusions as well
		log.Info("Setting voting config exclusions", "excluding", leavingMasters)
		if err := esClient.AddVotingConfigExclusions(context.TODO(), leavingMasters, ""); err != nil {
			return err
		}
	}
	return nil
}
