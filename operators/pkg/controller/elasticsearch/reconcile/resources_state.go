// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconcile

import (
	"fmt"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/services"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
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
	CurrentPods pod.PodsWithConfig
	// CurrentPodsByPhase are all non-deleted Elasticsearch indexed by their PodPhase
	CurrentPodsByPhase map[corev1.PodPhase]pod.PodsWithConfig
	// DeletingPods are all deleted Elasticsearch pods.
	DeletingPods pod.PodsWithConfig
	// PVCs are all the PVCs related to this deployment.
	PVCs []corev1.PersistentVolumeClaim
	// ExternalService is the user-facing service related to the Elasticsearch cluster.
	ExternalService corev1.Service
}

// NewResourcesStateFromAPI reflects the current ResourcesState from the API
func NewResourcesStateFromAPI(c k8s.Client, es v1alpha1.Elasticsearch) (*ResourcesState, error) {
	labelSelector := label.NewLabelSelectorForElasticsearch(es)

	allPods, err := getPods(c, es, labelSelector, nil)
	if err != nil {
		return nil, err
	}

	deletingPods := make(pod.PodsWithConfig, 0)
	currentPods := make(pod.PodsWithConfig, 0, len(allPods))
	currentPodsByPhase := make(map[corev1.PodPhase]pod.PodsWithConfig, 0)
	// filter out pods scheduled for deletion
	for _, p := range allPods {
		if p.DeletionTimestamp != nil {
			deletingPods = append(deletingPods, podWithConfig)
			continue
		}

		currentPods = append(currentPods, podWithConfig)

		podsInPhase, ok := currentPodsByPhase[p.Status.Phase]
		if !ok {
			podsInPhase = pod.PodsWithConfig{podWithConfig}
		} else {
			podsInPhase = append(podsInPhase, podWithConfig)
		}
		currentPodsByPhase[p.Status.Phase] = podsInPhase
	}

	pvcs, err := getPersistentVolumeClaims(c, es, labelSelector, nil)
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
		PVCs:               pvcs,
		ExternalService:    externalService,
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
	es v1alpha1.Elasticsearch,
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
	es v1alpha1.Elasticsearch,
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
