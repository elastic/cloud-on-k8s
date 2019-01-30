// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconcile

import (
	"fmt"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
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
	// PVCs are all the PVCs related to this deployment.
	PVCs []corev1.PersistentVolumeClaim
}

// NewResourcesStateFromAPI reflects the current ResourcesState from the API
func NewResourcesStateFromAPI(c k8s.Client, es v1alpha1.ElasticsearchCluster) (*ResourcesState, error) {
	labelSelector, err := label.NewLabelSelectorForElasticsearch(es)
	if err != nil {
		return nil, err
	}

	allPods, err := getPods(c, es, labelSelector, nil)
	if err != nil {
		return nil, err
	}

	deletingPods := make([]corev1.Pod, 0)
	currentPods := make([]corev1.Pod, 0, len(allPods))
	currentPodsByPhase := make(map[corev1.PodPhase][]corev1.Pod, 0)
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

	pvcs, err := getPersistentVolumeClaims(c, es, labelSelector, nil)

	state := ResourcesState{
		AllPods:            allPods,
		CurrentPods:        currentPods,
		CurrentPodsByPhase: currentPodsByPhase,
		DeletingPods:       deletingPods,
		PVCs:               pvcs,
	}

	return &state, nil
}

// FindPVCByName looks up a PVC by claim name.
func (state ResourcesState) FindPVCByName(name string) (corev1.PersistentVolumeClaim, error) {
	for _, pvc := range state.PVCs {
		if pvc.Name == name {
			return pvc, nil
		}
	}
	return corev1.PersistentVolumeClaim{}, fmt.Errorf("no PVC named %s found", name)
}

// getPods returns list of pods in the current namespace with a specific set of selectors.
func getPods(
	c k8s.Client,
	es v1alpha1.ElasticsearchCluster,
	labelSelectors labels.Selector,
	fieldSelectors fields.Selector,
) ([]corev1.Pod, error) {
	var podList corev1.PodList

	listOpts := client.ListOptions{
		Namespace:     es.Namespace,
		LabelSelector: labelSelectors,
		FieldSelector: fieldSelectors,
	}

	if err := c.List(&listOpts, &podList); err != nil {
		return nil, err
	}

	return podList.Items, nil
}

// getPersistentVolumeClaims returns a list of PVCs in the current namespace with a specific set of selectors.
func getPersistentVolumeClaims(
	c k8s.Client,
	es v1alpha1.ElasticsearchCluster,
	labelSelectors labels.Selector,
	fieldSelectors fields.Selector,
) ([]corev1.PersistentVolumeClaim, error) {
	var pvcs corev1.PersistentVolumeClaimList

	listOpts := client.ListOptions{
		Namespace:     es.Namespace,
		LabelSelector: labelSelectors,
		FieldSelector: fieldSelectors,
	}

	if err := c.List(&listOpts, &pvcs); err != nil {
		return nil, err
	}

	return pvcs.Items, nil
}
