// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/autoscaling"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"
)

// reconcileElasticsearch updates Elasticsearch NodeSets according to autoscaling recommendations.
// It updates NodeSet count and CPU/memory shorthand resources, adjusts storage when needed, and
// removes any CPU/memory entries the previous operator may have written to the main container in
// the NodeSet pod template.
func reconcileElasticsearch(
	log logr.Logger,
	es *esv1.Elasticsearch,
	nextClusterResources v1alpha1.ClusterResources,
) error {
	nextResourcesByNodeSet := nextClusterResources.ByNodeSet()
	for i := range es.Spec.NodeSets {
		name := es.Spec.NodeSets[i].Name
		nodeSetResources, ok := nextResourcesByNodeSet[name]
		if !ok {
			// No desired resources returned for this NodeSet, leave it untouched.
			log.V(1).Info("Skipping nodeset update", "nodeset", name)
			continue
		}

		// Compute the next shorthand resources. During operator upgrades from versions that wrote
		// autoscaled CPU/memory only in the PodTemplate container resources, this progressively
		// converges NodeSet.Resources to the autoscaler recommendation.
		currentResources := es.Spec.NodeSets[i].Resources
		nextResources := nodeSetResources.NodeResources.ToNodeSetResourcesWith(currentResources)

		// Strip CPU/memory entries from the main container in the pod template. Autoscaler-managed
		// NodeSets must have a single source of truth (NodeSet.Resources) for CPU/memory; leaving
		// stale values in the PodTemplate causes the validating webhook to emit an admission warning
		// on every reconcile of an existing autoscaled cluster.
		podTemplateChanged := stripAutoscaledResourcesFromPodTemplate(&es.Spec.NodeSets[i])

		// Only write to the NodeSet when something changed so the upstream Client.Update call
		// (in the controller) is a no-op and does not dirty the Elasticsearch custom resource
		// unnecessarily on every reconcile.
		if es.Spec.NodeSets[i].Count != nodeSetResources.NodeCount ||
			!apiequality.Semantic.DeepEqual(currentResources, nextResources) ||
			podTemplateChanged {
			es.Spec.NodeSets[i].Count = nodeSetResources.NodeCount
			es.Spec.NodeSets[i].Resources = nextResources
			log.V(1).Info("Updating nodeset with resources", "nodeset", name, "resources", nextClusterResources)
		}

		// Update storage
		if nodeSetResources.HasRequest(corev1.ResourceStorage) {
			nextStorage, err := newVolumeClaimTemplate(nodeSetResources.GetRequest(corev1.ResourceStorage), es.Spec.NodeSets[i])
			if err != nil {
				return err
			}
			es.Spec.NodeSets[i].VolumeClaimTemplates = nextStorage
		}
	}
	return nil
}

// stripAutoscaledResourcesFromPodTemplate removes CPU and memory entries from the Elasticsearch
// main container's Resources in the NodeSet pod template, returning true if anything was changed.
// Non-CPU/memory keys and ResourceClaims are preserved. Empty Requests/Limits maps are reset to
// nil so the LimitRange escape hatch in defaults.WithResourcesAndOverrides is not accidentally
// triggered by leftover empty-but-non-nil maps.
func stripAutoscaledResourcesFromPodTemplate(nodeSet *esv1.NodeSet) bool {
	changed := false
	for i := range nodeSet.PodTemplate.Spec.Containers {
		container := &nodeSet.PodTemplate.Spec.Containers[i]
		if container.Name != esv1.ElasticsearchContainerName {
			continue
		}
		for _, key := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
			if _, ok := container.Resources.Requests[key]; ok {
				delete(container.Resources.Requests, key)
				changed = true
			}
			if _, ok := container.Resources.Limits[key]; ok {
				delete(container.Resources.Limits, key)
				changed = true
			}
		}
		if len(container.Resources.Requests) == 0 && container.Resources.Requests != nil {
			container.Resources.Requests = nil
		}
		if len(container.Resources.Limits) == 0 && container.Resources.Limits != nil {
			container.Resources.Limits = nil
		}
		break
	}
	return changed
}

func newVolumeClaimTemplate(storageQuantity resource.Quantity, nodeSet esv1.NodeSet) ([]corev1.PersistentVolumeClaim, error) {
	onlyOneVolumeClaimTemplate, volumeClaimTemplate := autoscaling.HasAtMostOnePersistentVolumeClaim(nodeSet)
	if !onlyOneVolumeClaimTemplate {
		return nil, fmt.Errorf(autoscaling.UnexpectedVolumeClaimError)
	}
	if volumeClaimTemplate == nil {
		// Init a new volume claim template
		volumeClaimTemplate = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: volume.ElasticsearchDataVolumeName,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
			},
		}
	}
	// Adjust the size
	if volumeClaimTemplate.Spec.Resources.Requests == nil {
		volumeClaimTemplate.Spec.Resources.Requests = make(corev1.ResourceList)
	}
	volumeClaimTemplate.Spec.Resources.Requests[corev1.ResourceStorage] = storageQuantity
	return []corev1.PersistentVolumeClaim{*volumeClaimTemplate}, nil
}
