// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package comparison

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	corev1 "k8s.io/api/core/v1"
)

func PodMatchesSpec(
	es v1alpha1.Elasticsearch,
	podWithConfig pod.PodWithConfig,
	spec pod.PodSpecContext,
	state reconcile.ResourcesState,
) (bool, []string, error) {
	pod := podWithConfig.Pod
	config := podWithConfig.Config

	comparisons := []Comparison{
		// -- pod meta
		// require same namespace
		NewStringComparison(es.Namespace, pod.Namespace, "Pod namespace"),
		// require same base pod name
		NewStringComparison(name.Basename(name.NewPodName(es.Name, spec.NodeSpec)), name.Basename(pod.Name), "Pod base name"),
		// require spec labels and annotations to be present on the actual pod (which can have more)
		MapSubsetComparison(spec.NodeSpec.PodTemplate.Labels, pod.Labels, "Labels mismatch"),
		MapSubsetComparison(spec.NodeSpec.PodTemplate.Annotations, pod.Annotations, "Annotations mismatch"),

		// -- pod spec
		// require strict spec equality
		ComparePodSpec(spec.PodSpec, pod),
		// require pvc compatibility
		comparePersistentVolumeClaims(pod.Spec.Volumes, spec.NodeSpec.VolumeClaimTemplates, state),

		// -- config
		// require strict equality
		compareConfigs(config, spec.Config),
	}

	for _, c := range comparisons {
		if !c.Match {
			return false, c.MismatchReasons, nil
		}
	}

	return true, nil, nil
}

// ComparePodSpec returns a ComparisonMatch if the given spec matches the spec of the given pod.
// Comparison is based on the hash of the pod spec (before resource creation), stored in a label in the pod.
func ComparePodSpec(spec corev1.PodSpec, existingPod corev1.Pod) Comparison {
	existingPodHash := hash.GetSpecHashLabel(existingPod.Labels)
	if existingPodHash == "" {
		return ComparisonMismatch(fmt.Sprintf("No %s label set on the existing pod", hash.SpecHashLabelName))
	}
	if hash.HashObject(spec) != existingPodHash {
		return ComparisonMismatch("Spec hash and running pod spec hash are not equal")
	}
	return ComparisonMatch
}

// MapSubsetComparison returns ComparisonMatch if the expected labels (keys and values) are present in the
// actual map. The actual map may contain more entries: this will not cause a mismatch.
// This allows user to add additional labels or annotations to pods, while not causing the pod to be replaced.
func MapSubsetComparison(expected map[string]string, actual map[string]string, mismatchMsg string) Comparison {
	for k, expectedValue := range expected {
		actualValue, exists := actual[k]
		if !exists {
			return ComparisonMismatch(fmt.Sprintf("%s: %s does not exist", mismatchMsg, k))
		}
		if actualValue != expectedValue {
			return ComparisonMismatch(fmt.Sprintf(
				"%s: value for %s (%s) does not match the expected one (%s)",
				mismatchMsg, k, actualValue, expectedValue))
		}
	}
	return ComparisonMatch
}
