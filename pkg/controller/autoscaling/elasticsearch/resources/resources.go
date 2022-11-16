// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package resources

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"

	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
)

// Match returns true if the resources assigned to a container in a NodeSet matches the one specified in the NodeSetsResources.
// It also returns false if the container is not found in the NodeSet.
func Match(ntr v1alpha1.NodeSetsResources, containerName string, nodeSet esv1.NodeSet) (bool, error) {
	for _, nodeSetNodeCount := range ntr.NodeSetNodeCount {
		if nodeSetNodeCount.Name != nodeSet.Name {
			continue
		}
		if nodeSetNodeCount.NodeCount != nodeSet.Count {
			// The number of nodes in the NodeSetsResources and in the nodeSet is not equal.
			return false, nil
		}

		// Compare volume request
		switch len(nodeSet.VolumeClaimTemplates) {
		case 0:
			// If there is no VolumeClaimTemplate in the NodeSet then there should be no storage request in the NodeSetsResources.
			if ntr.HasRequest(corev1.ResourceStorage) {
				return false, nil
			}
		case 1:
			volumeClaim := nodeSet.VolumeClaimTemplates[0]
			if !ResourceEqual(corev1.ResourceStorage, ntr.NodeResources.Requests, volumeClaim.Spec.Resources.Requests) {
				return false, nil
			}
		default:
			return false, fmt.Errorf("only 1 volume claim template is allowed when autoscaling is enabled, got %d in nodeSet %s", len(nodeSet.VolumeClaimTemplates), nodeSet.Name)
		}

		// Compare CPU and Memory requests
		container := getContainer(containerName, nodeSet.PodTemplate.Spec.Containers)
		if container == nil {
			return false, nil
		}
		return ResourceEqual(corev1.ResourceMemory, ntr.NodeResources.Requests, container.Resources.Requests) &&
			ResourceEqual(corev1.ResourceCPU, ntr.NodeResources.Requests, container.Resources.Requests) &&
			ResourceEqual(corev1.ResourceMemory, ntr.NodeResources.Limits, container.Resources.Limits) &&
			ResourceEqual(corev1.ResourceCPU, ntr.NodeResources.Limits, container.Resources.Limits), nil
	}
	return false, nil
}

func ResourceEqual(resourceName corev1.ResourceName, expected, current corev1.ResourceList) bool {
	if len(expected) == 0 {
		// No value expected, return true
		return true
	}
	expectedValue, hasExpectedValue := expected[resourceName]
	if !hasExpectedValue {
		// Expected values does not contain the resource
		return true
	}
	if len(current) == 0 {
		// Value is expected but current is nil or empty
		return false
	}
	currentValue, hasCurrentValue := current[resourceName]
	if !hasCurrentValue {
		// Current values does not contain the resource
		return false
	}
	return expectedValue.Equal(currentValue)
}

func getContainer(name string, containers []corev1.Container) *corev1.Container {
	for i := range containers {
		container := containers[i]
		if container.Name == name {
			// Remove the container
			return &container
		}
	}
	return nil
}
