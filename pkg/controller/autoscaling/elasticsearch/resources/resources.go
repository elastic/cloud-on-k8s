// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package resources

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1alpha1"

	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
)

// Match returns true if the NodeSet count and resources match the ones specified in NodeSetsResources.
func Match(ntr v1alpha1.NodeSetsResources, nodeSet esv1.NodeSet) (bool, error) {
	for _, nodeSetNodeCount := range ntr.NodeSetNodeCount {
		if nodeSetNodeCount.Name != nodeSet.Name {
			continue
		}
		if nodeSetNodeCount.NodeCount != nodeSet.Count {
			// The number of nodes in the NodeSetsResources and in the nodeSet is not equal.
			return false, nil
		}

		// A NodeSet managed by an autoscaling policy is allowed at most one volume claim, because the
		// data deciders do not support multiple data paths.
		if len(nodeSet.VolumeClaimTemplates) > 1 {
			return false, fmt.Errorf("only 1 volume claim template is allowed when autoscaling is enabled, got %d in nodeSet %s", len(nodeSet.VolumeClaimTemplates), nodeSet.Name)
		}

		// Compare the storage request. The autoscaling controller writes the size to the NodeSet
		// storage shorthand rather than into the volume claim template, so that is where the
		// recommendation has to be reflected.
		if !ResourceEqual(corev1.ResourceStorage, ntr.NodeResources.Requests, storageRequestOf(nodeSet)) {
			return false, nil
		}

		currentRequests := nodeSet.Resources.Requests.ToResourceList()
		currentLimits := nodeSet.Resources.Limits.ToResourceList()
		return ResourceEqual(corev1.ResourceMemory, ntr.NodeResources.Requests, currentRequests) &&
			ResourceEqual(corev1.ResourceCPU, ntr.NodeResources.Requests, currentRequests) &&
			ResourceEqual(corev1.ResourceMemory, ntr.NodeResources.Limits, currentLimits) &&
			ResourceEqual(corev1.ResourceCPU, ntr.NodeResources.Limits, currentLimits), nil
	}
	return false, nil
}

// storageRequestOf returns the NodeSet's effective storage request as a ResourceList, so it can be
// compared with a recommendation. The storage shorthand wins, and the volume claim template's own
// request is the fallback for a NodeSet that has not been reconciled by the autoscaler yet.
//
// If resources.storage is removed from the spec by hand, the shorthand is nil and the VCT becomes
// the fallback. That causes Match to return false again, so the autoscaler refills the shorthand on
// the next reconcile via StorageRequestOr.
func storageRequestOf(nodeSet esv1.NodeSet) corev1.ResourceList {
	if nodeSet.Resources.Storage != nil {
		return corev1.ResourceList{corev1.ResourceStorage: *nodeSet.Resources.Storage}
	}
	if len(nodeSet.VolumeClaimTemplates) == 1 {
		q, ok := nodeSet.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
		if ok {
			return corev1.ResourceList{corev1.ResourceStorage: q}
		}
	}
	return nil
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
