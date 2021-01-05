// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconcile

import (
	"context"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	// ExternalService is the user-facing service related to the Elasticsearch cluster.
	ExternalService corev1.Service
}

// NewResourcesStateFromAPI reflects the current ResourcesState from the API
func NewResourcesStateFromAPI(c k8s.Client, es esv1.Elasticsearch) (*ResourcesState, error) {
	labelSelector := label.NewLabelSelectorForElasticsearch(es)

	allPods, err := getPods(c, es, labelSelector)
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

	externalService, err := services.GetExternalService(c, es)
	if err != nil {
		return nil, err
	}

	state := ResourcesState{
		AllPods:            allPods,
		CurrentPods:        currentPods,
		CurrentPodsByPhase: currentPodsByPhase,
		DeletingPods:       deletingPods,
		StatefulSets:       ssets,
		ExternalService:    externalService,
	}

	return &state, nil
}

// getPods returns list of pods in the current namespace with a specific set of selectors.
func getPods(
	c k8s.Client,
	es esv1.Elasticsearch,
	labelSelector client.MatchingLabels,
) ([]corev1.Pod, error) {
	var podList corev1.PodList
	ns := client.InNamespace(es.Namespace)

	if err := c.List(context.Background(), &podList, ns, labelSelector); err != nil {
		return nil, err
	}

	return podList.Items, nil
}
