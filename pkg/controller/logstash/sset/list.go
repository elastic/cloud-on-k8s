// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package sset

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/set"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/stringsutil"
)

type StatefulSetList []appsv1.StatefulSet

// RetrieveActualStatefulSets returns the list of existing StatefulSets labeled for the given es cluster.
// It is sorted using a natural order sort so that algorithms which are using the resulting list are more predictable and stable.
func RetrieveActualStatefulSets(c k8s.Client, ls types.NamespacedName) (StatefulSetList, error) {
	var ssets appsv1.StatefulSetList
	ns := client.InNamespace(ls.Namespace)
	matchLabels := labels.NewLabelSelectorForLogstashName(ls.Name)
	err := c.List(context.Background(), &ssets, ns, matchLabels)
	sort.Slice(ssets.Items, func(i, j int) bool {
		return ssets.Items[i].Name < ssets.Items[j].Name
	})
	return StatefulSetList(ssets.Items), err
}

// GetByName returns the StatefulSet with the given name, and a bool indicating if the StatefulSet was found.
func (l StatefulSetList) GetByName(ssetName string) (appsv1.StatefulSet, bool) {
	for _, sset := range l {
		if sset.Name == ssetName {
			return sset, true
		}
	}
	return appsv1.StatefulSet{}, false
}
//
// Names returns the set of StatefulSets names.
func (l StatefulSetList) Names() set.StringSet {
	names := set.Make()
	for _, statefulSet := range l {
		names.Add(statefulSet.Name)
	}
	return names
}

// PodReconciliationDone returns true if actual existing pods match what is specified in the StatefulSetList.
// It may return false if there are pods in the process of being:
// - created (but not there in our resources cache)
// - removed (but still there in our resources cache)
// Status of the pods (running, error, etc.) is ignored.
func (l StatefulSetList) PodReconciliationDone(ctx context.Context, c k8s.Client) (bool, string, error) {
	for _, statefulSet := range l {
		pendingCreations, pendingDeletions, err := pendingPodsForStatefulSet(ctx, c, statefulSet)
		if err != nil {
			return false, "", err
		}
		if len(pendingCreations) > 0 || len(pendingDeletions) > 0 {
			ulog.FromContext(ctx).V(1).Info(
				"Some pods still need to be created/deleted",
				"namespace", statefulSet.Namespace, "statefulset_name", statefulSet.Name,
				"pending_creations", pendingCreations, "pending_deletions", pendingDeletions,
			)

			var reason strings.Builder
			reason.WriteString(fmt.Sprintf("StatefulSet %s has pending Pod operations", statefulSet.Name))
			if len(pendingCreations) > 0 {
				reason.WriteString(fmt.Sprintf(", creations: %s", pendingCreations))
			}
			if len(pendingDeletions) > 0 {
				reason.WriteString(fmt.Sprintf(", deletions: %s", pendingDeletions))
			}

			return false, reason.String(), nil
		}
	}
	return true, "", nil
}

func pendingPodsForStatefulSet(ctx context.Context, c k8s.Client, statefulSet appsv1.StatefulSet) ([]string, []string, error) {
	// check all expected pods are there: no more, no less
	actualPods, err := GetActualPodsForStatefulSet(c, k8s.ExtractNamespacedName(&statefulSet))
	if err != nil {
		return nil, nil, err
	}
	actualPodNames := k8s.PodNames(actualPods)
	expectedPodNames := PodNames(statefulSet)
	pendingCreations, pendingDeletions := stringsutil.Difference(expectedPodNames, actualPodNames)
	return pendingCreations, pendingDeletions, nil
}

// PendingReconciliation returns the list of StatefulSets for which status.observedGeneration does not match the metadata.generation.
// The status is automatically updated by the StatefulSet controller: if the observedGeneration does not match
// the metadata generation, it means the resource has not been processed by the StatefulSet controller yet.
// When that happens, other fields in the StatefulSet status (eg. "updateRevision") may not be up to date.
func (l StatefulSetList) PendingReconciliation() StatefulSetList {
	var statefulSetList StatefulSetList
	for _, s := range l {
		if s.Generation != s.Status.ObservedGeneration {
			s := s
			statefulSetList = append(statefulSetList, s)
		}
	}
	return statefulSetList
}