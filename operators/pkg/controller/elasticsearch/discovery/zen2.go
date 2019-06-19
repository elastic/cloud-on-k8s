// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package discovery

import (
	"context"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	corev1 "k8s.io/api/core/v1"
)

func Zen2SetVotingExclusions(esClient client.Client, deletingPods []corev1.Pod) error {
	leavingMasters := make([]string, 0)

	for _, deletingPod := range deletingPods {
		if label.IsMasterNode(deletingPod) {
			leavingMasters = append(leavingMasters, deletingPod.Name)
		}
	}

	if len(leavingMasters) != 0 {
		// TODO: only update if required and remove old exclusions as well
		log.Info("Setting voting config exclusions", "excluding", leavingMasters)
		ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
		defer cancel()
		if err := esClient.AddVotingConfigExclusions(ctx, leavingMasters, ""); err != nil {
			return err
		}
	} else {
		// nothing to exclude
		log.Info("Deleting voting config exclusions")

		ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
		defer cancel()

		// TODO: only update if there's changes to the voting exclusions
		if err := esClient.DeleteVotingConfigExclusions(ctx, false); err != nil {
			return err
		}
	}

	return nil
}
