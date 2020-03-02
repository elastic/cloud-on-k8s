// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen2

import (
	"context"
	"sort"
	"strings"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var (
	log = logf.Log.WithName("zen2")
)

const (
	// VotingConfigExclusionsAnnotationName is an annotation that stores the last applied voting config exclusions.
	// An empty value means no voting config exclusions are set.
	VotingConfigExclusionsAnnotationName = "elasticsearch.k8s.elastic.co/voting-config-exclusions"
)

// serializeExcludedNodesForAnnotation returns a sorted comma-separated representation of the given slice.
func serializeExcludedNodesForAnnotation(excludedNodes []string) string {
	// sort a copy to not mutate the given slice
	sliceCopy := make([]string, len(excludedNodes))
	copy(sliceCopy, excludedNodes)
	sort.Strings(sliceCopy)
	return strings.Join(sliceCopy, ",")
}

// votingConfigAnnotationMatches returns true if the voting config exclusions annotation value
// matches the given excluded nodes.
func votingConfigAnnotationMatches(es esv1.Elasticsearch, excludedNodes []string) bool {
	value, exists := es.Annotations[VotingConfigExclusionsAnnotationName]
	if !exists {
		return false
	}
	return value == serializeExcludedNodesForAnnotation(excludedNodes)
}

// setVotingConfigAnnotation sets the value of the voting config exclusions annotation to the given excluded nodes.
func setVotingConfigAnnotation(c k8s.Client, es esv1.Elasticsearch, excludedNodes []string) error {
	if es.Annotations == nil {
		es.Annotations = map[string]string{}
	}
	es.Annotations[VotingConfigExclusionsAnnotationName] = serializeExcludedNodesForAnnotation(excludedNodes)
	return c.Update(&es)
}

// AddToVotingConfigExclusions adds the given node names to exclude from voting config exclusions.
func AddToVotingConfigExclusions(ctx context.Context, c k8s.Client, esClient client.Client, es esv1.Elasticsearch, excludeNodes []string) error {
	compatible, err := AllMastersCompatibleWithZen2(c, es)
	if err != nil {
		return err
	}
	if !compatible {
		return nil
	}

	if votingConfigAnnotationMatches(es, excludeNodes) {
		// nothing to do, we already applied that setting
		return nil
	}

	log.Info("Setting voting config exclusions", "namespace", es.Namespace, "nodes", excludeNodes)
	ctx, cancel := context.WithTimeout(ctx, client.DefaultReqTimeout)
	defer cancel()
	if err := esClient.AddVotingConfigExclusions(ctx, excludeNodes, ""); err != nil {
		return err
	}
	// store the excluded nodes value in an annotation so we don't perform the same API call over and over again
	return setVotingConfigAnnotation(c, es, excludeNodes)
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
	return actualStatefulSets.PodReconciliationDone(c)
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

	var noExcludedNodes []string = nil
	if votingConfigAnnotationMatches(es, noExcludedNodes) {
		// nothing to do, we already applied that setting
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

	ctx, cancel := context.WithTimeout(ctx, client.DefaultReqTimeout)
	defer cancel()
	log.Info("Ensuring no voting exclusions are set", "namespace", es.Namespace, "es_name", es.Name)
	if err := esClient.DeleteVotingConfigExclusions(ctx, false); err != nil {
		return false, err
	}

	// store the excluded nodes value in an annotation so we don't perform the same API call over and over again
	return false, setVotingConfigAnnotation(c, es, noExcludedNodes)
}
