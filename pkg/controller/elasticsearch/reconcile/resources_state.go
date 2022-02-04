// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package reconcile

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// ResourcesState contains information about a deployments resources.
type ResourcesState struct {
	// AllPods are all the Elasticsearch pods related to the Elasticsearch cluster, including ones with a
	// DeletionTimestamp tombstone set.
	AllPods []corev1.Pod
	// CurrentPods are all non-deleted Elasticsearch pods.
	CurrentPods []corev1.Pod
	// CurrentPodsByPhase are all non-deleted Elasticsearch indexed by their PodPhase
	CurrentPodsByPhase map[corev1.PodPhase][]corev1.Pod
	// DeletingPods are all deleted Elasticsearch pods.
	DeletingPods []corev1.Pod
	// StatefulSets are all existing StatefulSets for the cluster.
	StatefulSets sset.StatefulSetList
}

// NewResourcesStateFromAPI reflects the current ResourcesState from the API
func NewResourcesStateFromAPI(c k8s.Client, es esv1.Elasticsearch) (*ResourcesState, error) {
	allPods, err := k8s.PodsMatchingLabels(c, es.Namespace, label.NewLabelSelectorForElasticsearch(es))
	if err != nil {
		return nil, err
	}

	deletingPods := make([]corev1.Pod, 0)
	currentPods := make([]corev1.Pod, 0, len(allPods))
	currentPodsByPhase := make(map[corev1.PodPhase][]corev1.Pod)
	// filter out pods scheduled for deletion
	for _, p := range allPods {
		if p.DeletionTimestamp != nil {
			deletingPods = append(deletingPods, p)
			continue
		}

		currentPods = append(currentPods, p)

		podsInPhase, ok := currentPodsByPhase[p.Status.Phase]
		if !ok {
			podsInPhase = []corev1.Pod{p}
		} else {
			podsInPhase = append(podsInPhase, p)
		}
		currentPodsByPhase[p.Status.Phase] = podsInPhase
	}

	ssets, err := sset.RetrieveActualStatefulSets(c, types.NamespacedName{Namespace: es.Namespace, Name: es.Name})
	if err != nil {
		return nil, err
	}

	state := ResourcesState{
		AllPods:            allPods,
		CurrentPods:        currentPods,
		CurrentPodsByPhase: currentPodsByPhase,
		DeletingPods:       deletingPods,
		StatefulSets:       ssets,
	}

	return &state, nil
}

// AvailableElasticsearchNodes filters a slice of pods for the ones that are ready.
func AvailableElasticsearchNodes(pods []corev1.Pod) []corev1.Pod {
	var nodesAvailable []corev1.Pod
	for _, pod := range pods {
		if k8s.IsPodReady(pod) {
			nodesAvailable = append(nodesAvailable, pod)
		}
	}
	return nodesAvailable
}
