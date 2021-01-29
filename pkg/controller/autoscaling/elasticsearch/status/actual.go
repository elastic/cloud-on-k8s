// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package status

import (
	"context"
	"fmt"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/resources"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ImportExistingResources attempts to infer the resources to use in a tier if an autoscaling policy  is not in the Status.
// It can be the case if:
//  * The cluster was manually managed and the user wants to manage resources with the autoscaling controller. In that case
//    we want to be able to set some good default resources even if the autoscaling API is not responding.
// * The Elasticsearch has been replaced and the status annotation has been lost.
func (s *Status) ImportExistingResources(
	log logr.Logger,
	c k8s.Client,
	as esv1.AutoscalingSpec,
	namedTiers esv1.AutoscaledNodeSets,
) error {
	for _, autoscalingPolicy := range as.AutoscalingPolicySpecs {
		if _, inStatus := s.GetNamedTierResources(autoscalingPolicy.Name); inStatus {
			// This autoscaling policy is already managed and we have some resources in the Status.
			continue
		}
		// Get the nodeSets
		nodeSetList, exists := namedTiers[autoscalingPolicy.Name]
		if !exists {
			// Not supposed to happen with a proper validation in place, but we still want to report this error
			return fmt.Errorf("no nodeSet associated to autoscaling policy %s", autoscalingPolicy.Name)
		}
		resources, err := namedTierResourcesFromStatefulSets(c, as.Elasticsearch, autoscalingPolicy, nodeSetList.Names())
		if err != nil {
			return err
		}
		if resources == nil {
			// No StatefulSet, the cluster or the tier might be a new one.
			continue
		}
		log.Info("Importing resources from existing StatefulSets",
			"policy", autoscalingPolicy.Name,
			"nodeset", resources.NodeSetNodeCount,
			"count", resources.NodeSetNodeCount.TotalNodeCount(),
			"resources", resources.ToInt64(),
		)
		// We only want to save the status the resources
		s.AutoscalingPolicyStatuses = append(s.AutoscalingPolicyStatuses,
			AutoscalingPolicyStatus{
				Name:                   autoscalingPolicy.Name,
				NodeSetNodeCount:       resources.NodeSetNodeCount,
				ResourcesSpecification: resources.NodeResources,
			})
	}
	return nil
}

// namedTierResourcesFromStatefulSets creates NodeSetsResources from existing StatefulSets
func namedTierResourcesFromStatefulSets(
	c k8s.Client,
	es esv1.Elasticsearch,
	autoscalingPolicySpec esv1.AutoscalingPolicySpec,
	nodeSets []string,
) (*resources.NodeSetsResources, error) {
	namedTierResources := resources.NodeSetsResources{
		Name: autoscalingPolicySpec.Name,
	}
	found := false
	// For each nodeSet:
	// 1. we try to get the corresponding StatefulSet
	// 2. we build a NodeSetsResources from the max. resources of each StatefulSet
	for _, nodeSetName := range nodeSets {
		statefulSetName := esv1.StatefulSet(es.Name, nodeSetName)
		statefulSet := appsv1.StatefulSet{}
		err := c.Get(
			context.Background(),
			client.ObjectKey{
				Namespace: es.Namespace,
				Name:      statefulSetName,
			}, &statefulSet)
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}

		found = true
		namedTierResources.NodeSetNodeCount = append(namedTierResources.NodeSetNodeCount, resources.NodeSetNodeCount{
			Name:      nodeSetName,
			NodeCount: getStatefulSetReplicas(statefulSet),
		})

		// Get data volume volume size
		ssetStorageRequest, err := getElasticsearchDataVolumeQuantity(statefulSet)
		if err != nil {
			return nil, err
		}
		if ssetStorageRequest != nil && autoscalingPolicySpec.IsStorageDefined() {
			if namedTierResources.HasRequest(corev1.ResourceStorage) {
				if ssetStorageRequest.Cmp(namedTierResources.GetRequest(corev1.ResourceStorage)) > 0 {
					namedTierResources.SetRequest(corev1.ResourceStorage, *ssetStorageRequest)
				}
			} else {
				namedTierResources.SetRequest(corev1.ResourceStorage, *ssetStorageRequest)
			}
		}

		// Get the memory and the CPU if any
		container := getContainer(esv1.ElasticsearchContainerName, statefulSet.Spec.Template.Spec.Containers)
		if container == nil {
			continue
		}
		if autoscalingPolicySpec.IsMemoryDefined() {
			namedTierResources.MaxMerge(container.Resources, corev1.ResourceMemory)
		}
		if autoscalingPolicySpec.IsCPUDefined() {
			namedTierResources.MaxMerge(container.Resources, corev1.ResourceCPU)
		}
	}
	if !found {
		return nil, nil
	}
	return &namedTierResources, nil
}

// getElasticsearchDataVolumeQuantity returns the volume claim quantity for the esv1.ElasticsearchDataVolumeName volume
func getElasticsearchDataVolumeQuantity(statefulSet appsv1.StatefulSet) (*resource.Quantity, error) {
	if len(statefulSet.Spec.VolumeClaimTemplates) > 1 {
		// We do not support nodeSets with more than one volume.
		return nil, fmt.Errorf("autoscaling does not support nodeSet with more than one volume claim")
	}

	if len(statefulSet.Spec.VolumeClaimTemplates) == 1 {
		volumeClaimTemplate := statefulSet.Spec.VolumeClaimTemplates[0]
		ssetStorageRequest, ssetHasStorageRequest := volumeClaimTemplate.Spec.Resources.Requests[corev1.ResourceStorage]
		if ssetHasStorageRequest {
			return &ssetStorageRequest, nil
		}
	}
	return nil, nil
}

func getStatefulSetReplicas(sset appsv1.StatefulSet) int32 {
	if sset.Spec.Replicas != nil {
		return *sset.Spec.Replicas
	}
	return 0
}

func getContainer(containerName string, containers []corev1.Container) *corev1.Container {
	for _, container := range containers {
		if container.Name == containerName {
			return &container
		}
	}
	return nil
}
