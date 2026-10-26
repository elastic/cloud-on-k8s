// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package zen2

import (
	"context"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	ssetcommon "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// AddToVotingConfigExclusions adds the given node names to exclude from voting config exclusions.
func AddToVotingConfigExclusions(ctx context.Context, esClient client.Client, es esv1.Elasticsearch, excludeNodes []string) error {
	ulog.FromContext(ctx).Info("Setting voting config exclusions", "namespace", es.Namespace, "nodes", excludeNodes)
	return esClient.AddVotingConfigExclusions(ctx, excludeNodes)
}

// canClearVotingConfigExclusions returns true if it is safe to clear voting config exclusions.
func canClearVotingConfigExclusions(ctx context.Context, c k8s.Client, actualStatefulSets sset.StatefulSetList) (bool, error) {
	// Voting config exclusions are set before master nodes are removed on sset downscale.
	// They can be cleared when:
	// - nodes are effectively removed
	// - nodes are expected to be in the cluster (shouldn't be removed anymore)
	// They cannot be cleared when:
	// - expected nodes to remove are not removed yet
	// - expectation like Pod being restarted should be check prior to calling this function
	// Pods still to be created are ignored, as they may never be, for example if a resource quota rejects them: waiting
	// would keep the exclusions until Elasticsearch rejects new ones at its limit. A leaving master whose Pod is
	// being recreated is excluded again by the downscale before it is removed.
	// Pods still to be deleted are not ignored: they may be excluded nodes that are not removed yet.
	for _, statefulSet := range actualStatefulSets {
		_, pendingDeletions, err := ssetcommon.PendingPodsForStatefulSet(c, statefulSet, label.StatefulSetNameLabelName)
		if err != nil {
			return false, err
		}
		if len(pendingDeletions) > 0 {
			ulog.FromContext(ctx).V(1).Info("Some pods still need to be deleted",
				"namespace", statefulSet.Namespace, "statefulset_name", statefulSet.Name, "pending_deletions", pendingDeletions)
			return false, nil
		}
	}
	return true, nil
}

// ClearVotingConfigExclusions resets the voting config exclusions if all excluded nodes are properly removed.
// It returns true if this should be retried later (re-queued).
func ClearVotingConfigExclusions(ctx context.Context, es esv1.Elasticsearch, c k8s.Client, esClient client.Client, actualStatefulSets sset.StatefulSetList) (bool, error) {
	log := ulog.FromContext(ctx)

	canClear, err := canClearVotingConfigExclusions(ctx, c, actualStatefulSets)
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
