// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package status

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// ImportExistingResources attempts to infer the resources to allocate to node sets if an autoscaling policy is not in the Status.
// It can be the case if:
// - The cluster was manually managed and the user wants to manage resources with the autoscaling controller. In that case
// we want to be able to set some good default resources even if the autoscaling API is not responding.
// - The Elasticsearch resource has been replaced and the status annotation has been lost in the process.
func ImportExistingResources(
	log logr.Logger,
	c k8s.Client,
	autoscalingPolicies v1alpha1.AutoscalingPolicySpecs,
	es esv1.Elasticsearch,
	autoscaledNodeSets esv1.AutoscaledNodeSets,
	status *v1alpha1.ElasticsearchAutoscalerStatus,
) error {
	for _, autoscalingPolicy := range autoscalingPolicies {
		if _, inStatus := status.CurrentResourcesForPolicy(autoscalingPolicy.Name); inStatus {
			// This autoscaling policy is already managed and we have some resources in the Status.
			continue
		}
		// Get the nodeSets
		nodeSetList, exists := autoscaledNodeSets[autoscalingPolicy.Name]
		if !exists {
			// Not supposed to happen with a proper validation in place, but we still want to report this error
			return fmt.Errorf("no nodeSet associated to autoscaling policy %s", autoscalingPolicy.Name)
		}
		resources, err := nodeSetsResourcesResourcesFromStatefulSets(c, es, autoscalingPolicy, nodeSetList.Names())
		if err != nil {
			return err
		}
		if resources == nil {
			// No StatefulSet, the cluster or the autoscaling policy might be a new one.
			continue
		}
		log.Info("Importing resources from existing StatefulSets",
			"policy", autoscalingPolicy.Name,
			"nodeset", resources.NodeSetNodeCount,
			"count", resources.NodeSetNodeCount.TotalNodeCount(),
			"resources", resources.ToInt64(),
		)
		// We only want to save the status the resources
		status.AutoscalingPolicyStatuses = append(status.AutoscalingPolicyStatuses,
			v1alpha1.AutoscalingPolicyStatus{
				Name:                   autoscalingPolicy.Name,
				NodeSetNodeCount:       resources.NodeSetNodeCount,
				ResourcesSpecification: resources.NodeResources,
			})
	}
	return nil
}

// nodeSetsResourcesResourcesFromStatefulSets creates NodeSetsResources from existing StatefulSets
func nodeSetsResourcesResourcesFromStatefulSets(
	c k8s.Client,
	es esv1.Elasticsearch,
	autoscalingPolicySpec v1alpha1.AutoscalingPolicySpec,
	nodeSets []string,
) (*v1alpha1.NodeSetsResources, error) {
	nodeSetsResources := v1alpha1.NodeSetsResources{
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
		nodeSetsResources.NodeSetNodeCount = append(nodeSetsResources.NodeSetNodeCount, v1alpha1.NodeSetNodeCount{
			Name:      nodeSetName,
			NodeCount: getStatefulSetReplicas(statefulSet),
		})

		// Get data volume volume size
		ssetStorageRequest, err := getElasticsearchDataVolumeQuantity(statefulSet)
		if err != nil {
			return nil, err
		}
		if ssetStorageRequest != nil && autoscalingPolicySpec.IsStorageDefined() {
			if nodeSetsResources.HasRequest(corev1.ResourceStorage) {
				if ssetStorageRequest.Cmp(nodeSetsResources.GetRequest(corev1.ResourceStorage)) > 0 {
					nodeSetsResources.SetRequest(corev1.ResourceStorage, *ssetStorageRequest)
				}
			} else {
				nodeSetsResources.SetRequest(corev1.ResourceStorage, *ssetStorageRequest)
			}
		}

		// Get the memory and the CPU if any
		container := getContainer(esv1.ElasticsearchContainerName, statefulSet.Spec.Template.Spec.Containers)
		if container == nil {
			continue
		}
		if autoscalingPolicySpec.IsMemoryDefined() {
			nodeSetsResources.MaxMerge(container.Resources, corev1.ResourceMemory)
		}
		if autoscalingPolicySpec.IsCPUDefined() {
			nodeSetsResources.MaxMerge(container.Resources, corev1.ResourceCPU)
		}
	}
	if !found {
		return nil, nil
	}
	return &nodeSetsResources, nil
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
