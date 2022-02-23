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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/pkg/utils/set"
)

var log = ulog.Log.WithName("statefulset")

type StatefulSetList []appsv1.StatefulSet

// RetrieveActualStatefulSets returns the list of existing StatefulSets labeled for the given es cluster.
// It is sorted using a natural order sort so that algorithms which are using the resulting list are more predictable and stable.
func RetrieveActualStatefulSets(c k8s.Client, es types.NamespacedName) (StatefulSetList, error) {
	var ssets appsv1.StatefulSetList
	ns := client.InNamespace(es.Namespace)
	matchLabels := label.NewLabelSelectorForElasticsearchClusterName(es.Name)
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

// Names returns the set of StatefulSets names.
func (l StatefulSetList) Names() set.StringSet {
	names := set.Make()
	for _, statefulSet := range l {
		names.Add(statefulSet.Name)
	}
	return names
}

// ToUpdate filters the StatefulSetList to the ones having an update revision scheduled.
func (l StatefulSetList) ToUpdate() StatefulSetList {
	toUpdate := StatefulSetList{}
	for _, s := range l {
		// When using an OnDelete strategy current revision is never reset to update revision.
		// Just looking that the revision to detect updates does therefore does not work when reverting
		// to a previous revision and gives constant false positives after an initial update.
		// Only updated replicas != replicas expresses the fact that an update is still pending.
		if s.Status.UpdatedReplicas != s.Status.Replicas {
			toUpdate = append(toUpdate, s)
		}
	}
	return toUpdate
}

// PodNames returns the names of the pods for all StatefulSets in the list.
func (l StatefulSetList) PodNames() []string {
	names := make([]string, 0, len(l))
	for _, s := range l {
		names = append(names, PodNames(s)...)
	}
	return names
}

// ExpectedNodeCount returns the sum of replicas of each StatefulSet in the StatefulSetList.
func (l StatefulSetList) ExpectedNodeCount() int32 {
	count := int32(0)
	for _, s := range l {
		count += GetReplicas(s)
	}
	return count
}

// ExpectedMasterNodesCount returns the number of master nodes expected from the StatefulSetList.
func (l StatefulSetList) ExpectedMasterNodesCount() int32 {
	count := int32(0)
	for _, s := range l {
		if label.IsMasterNodeSet(s) {
			count += GetReplicas(s)
		}
	}
	return count
}

// ExpectedDataNodesCount returns the number of data nodes expected from the StatefulSetList.
func (l StatefulSetList) ExpectedDataNodesCount() int32 {
	count := int32(0)
	for _, s := range l {
		if label.IsDataNodeSet(s) {
			count += GetReplicas(s)
		}
	}
	return count
}

// ExpectedIngestNodesCount returns the number of ingest nodes expected from the StatefulSetList.
func (l StatefulSetList) ExpectedIngestNodesCount() int32 {
	count := int32(0)
	for _, s := range l {
		if label.IsIngestNodeSet(s) {
			count += GetReplicas(s)
		}
	}
	return count
}

// PVCNames returns the names of PVCs for all pods of the StatefulSetList.
func (l StatefulSetList) PVCNames() []string {
	var pvcNames []string
	for _, s := range l {
		podNames := PodNames(s)
		for _, claim := range s.Spec.VolumeClaimTemplates {
			for _, podName := range podNames {
				pvcNames = append(pvcNames, fmt.Sprintf("%s-%s", claim.Name, podName))
			}
		}
	}
	return pvcNames
}

// GetActualPods returns the list of pods currently existing in the StatefulSetList.
func (l StatefulSetList) GetActualPods(c k8s.Client) ([]corev1.Pod, error) {
	allPods := []corev1.Pod{}
	for _, statefulSet := range l {
		sset := statefulSet
		pods, err := GetActualPodsForStatefulSet(c, k8s.ExtractNamespacedName(&sset))
		if err != nil {
			return nil, err
		}
		allPods = append(allPods, pods...)
	}
	return allPods, nil
}

// PodReconciliationDone returns true if actual existing pods match what is specified in the StatefulSetList.
// It may return false if there are pods in the process of being:
// - created (but not there in our resources cache)
// - removed (but still there in our resources cache)
// Status of the pods (running, error, etc.) is ignored.
func (l StatefulSetList) PodReconciliationDone(c k8s.Client) (bool, string, error) {
	for _, statefulSet := range l {
		pendingCreations, pendingDeletions, err := pendingPodsForStatefulSet(c, statefulSet)
		if err != nil {
			return false, "", err
		}
		if len(pendingCreations) > 0 || len(pendingDeletions) > 0 {
			log.V(1).Info(
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

// DeepCopy returns a copy of the StatefulSetList with no reference to the original StatefulSetList.
func (l StatefulSetList) DeepCopy() StatefulSetList {
	result := make(StatefulSetList, 0, len(l))
	for _, s := range l {
		result = append(result, *s.DeepCopy())
	}
	return result
}

// WithStatefulSet returns the StatefulSetList updated to contain the given StatefulSet.
// If one already exists with the same namespace & name, it will be replaced.
func (l StatefulSetList) WithStatefulSet(statefulSet appsv1.StatefulSet) StatefulSetList {
	for i := range l {
		if l[i].Name == statefulSet.Name && l[i].Namespace == statefulSet.Namespace {
			// replace the existing StatefulSet in the list
			l[i] = statefulSet
			return l
		}
	}
	// add a new StatefulSet to the list
	return append(l, statefulSet)
}

// ESVersionMatch returns true if the ES version for this StatefulSet matches the given condition.
func ESVersionMatch(statefulSet appsv1.StatefulSet, condition func(v version.Version) bool) bool {
	v, err := GetESVersion(statefulSet)
	if err != nil {
		log.Error(err, "cannot parse version from StatefulSet", "namespace", statefulSet.Namespace, "name", statefulSet.Name)
		return false
	}
	return condition(v)
}

// AtLeastOneESVersionMatch returns true if at least one StatefulSet's ES version matches the given condition.
func AtLeastOneESVersionMatch(statefulSets StatefulSetList, condition func(v version.Version) bool) bool {
	for _, s := range statefulSets {
		if ESVersionMatch(s, condition) {
			return true
		}
	}
	return false
}
