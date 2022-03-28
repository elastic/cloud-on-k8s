// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package zen2

import (
	"context"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
)

var (
	log = ulog.Log.WithName("zen2")
)

// AddToVotingConfigExclusions adds the given node names to exclude from voting config exclusions.
func AddToVotingConfigExclusions(ctx context.Context, c k8s.Client, esClient client.Client, es esv1.Elasticsearch, excludeNodes []string) error {
	compatible, err := AllMastersCompatibleWithZen2(c, es)
	if err != nil {
		return err
	}
	if !compatible {
		return nil
	}

	log.Info("Setting voting config exclusions", "namespace", es.Namespace, "nodes", excludeNodes)
	return esClient.AddVotingConfigExclusions(context.Background(), excludeNodes)
}

// canClearVotingConfigExclusions returns true if it is safe to clear voting config exclusions.
func canClearVotingConfigExclusions(c k8s.Client, actualStatefulSets sset.StatefulSetList) (bool, error) {
	// Voting config exclusions are set before master nodes are removed on sset downscale.
	// They can be cleared when:
	// - nodes are effectively removed
	// - nodes are expected to be in the cluster (shouldn't be removed anymore)
	// They cannot be cleared when:
	// - expected nodes to remove are not removed yet
	// - expectation like Pod being restarted should be check prior to calling this function
	// PodReconciliationDone returns false is there are some pods not created yet: we don't really
	// care about those here, but that's still fine to requeue and retry later for the sake of simplicity.
	reconciled, _, err := actualStatefulSets.PodReconciliationDone(c)
	return reconciled, err
}

// ClearVotingConfigExclusions resets the voting config exclusions if all excluded nodes are properly removed.
// It returns true if this should be retried later (re-queued).
func ClearVotingConfigExclusions(ctx context.Context, es esv1.Elasticsearch, c k8s.Client, esClient client.Client, actualStatefulSets sset.StatefulSetList) (bool, error) {
	compatible, err := AllMastersCompatibleWithZen2(c, es)
	if err != nil {
		return false, err
	}
	if !compatible {
		// nothing to do
		return false, nil
	}

	canClear, err := canClearVotingConfigExclusions(c, actualStatefulSets)
	if err != nil {
		return false, err
	}
	if !canClear {
		log.V(1).Info("Cannot clear voting exclusions yet", "namespace", es.Namespace, "es_name", es.Name)
		return true, nil // requeue
	}

	log.Info("Ensuring no voting exclusions are set", "namespace", es.Namespace, "es_name", es.Name)
	return false, esClient.DeleteVotingConfigExclusions(ctx, false)
}
