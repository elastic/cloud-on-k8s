// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen2

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

var (
	log = logf.Log.WithName("zen2")
)

// AddToVotingConfigExclusions adds the given node names to exclude from voting config exclusions.
func AddToVotingConfigExclusions(esClient client.Client, sset appsv1.StatefulSet, excludeNodes []string) error {
	if !IsCompatibleWithZen2(sset) {
		return nil
	}
	log.Info("Setting voting config exclusions", "namespace", sset.Namespace, "nodes", excludeNodes)
	ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
	defer cancel()
	if err := esClient.AddVotingConfigExclusions(ctx, excludeNodes, ""); err != nil {
		return err
	}
	return nil
}

// canClearVotingConfigExclusions returns true if it is safe to clear voting config exclusions.
func canClearVotingConfigExclusions(c k8s.Client, es v1alpha1.Elasticsearch, actualStatefulSets sset.StatefulSetList) (bool, error) {
	// Voting config exclusions are set before master nodes are removed on sset downscale.
	// They can be cleared when:
	// - nodes are effectively removed
	// - nodes are expected to be in the cluster (shouldn't be removed anymore)
	// They cannot be cleared when:
	// - expected nodes to remove are not removed yet
	return actualStatefulSets.PodReconciliationDone(c, es)
}

// ClearVotingConfigExclusions resets the voting config exclusions if all excluded nodes are properly removed.
// It returns true if this should be retried later (re-queued).
func ClearVotingConfigExclusions(es v1alpha1.Elasticsearch, c k8s.Client, esClient client.Client, actualStatefulSets sset.StatefulSetList) (bool, error) {
	if !AtLeastOneNodeCompatibleWithZen2(actualStatefulSets) {
		return false, nil
	}
	canClear, err := canClearVotingConfigExclusions(c, es, actualStatefulSets)
	if err != nil {
		return false, err
	}
	if !canClear {
		log.V(1).Info("Cannot clear voting exclusions yet", "namespace", es.Namespace, "es_name", es.Name)
		return true, nil // requeue
	}

	ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
	defer cancel()
	log.Info("Ensuring no voting exclusions are set", "namespace", es.Namespace, "es_name", es.Name)
	if err := esClient.DeleteVotingConfigExclusions(ctx, false); err != nil {
		return false, err
	}
	return false, nil
}
