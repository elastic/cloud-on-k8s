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

// PodMatchesSpec compares an existing pod and its config with an expected pod spec, and returns true if the
// existing pod matches the expected pod spec, or returns a list of reasons why it does not match.
//
// A pod matches the spec if:
// - it has the same namespace and base name
// - it has the same configuration
// - it has the same PVC spec
// - it was created using the same pod template (whose hash is stored in the pod annotations)
func PodMatchesSpec(
	es v1alpha1.Elasticsearch,
	podWithConfig pod.PodWithConfig,
	spec pod.PodSpecContext,
	state reconcile.ResourcesState,
) (bool, []string, error) {
	pod := podWithConfig.Pod
	config := podWithConfig.Config

	comparisons := []Comparison{
		// require same namespace
		NewStringComparison(es.Namespace, pod.Namespace, "Pod namespace"),
		// require same base pod name
		NewStringComparison(name.Basename(name.NewPodName(es.Name, spec.NodeSpec)), name.Basename(pod.Name), "Pod base name"),
		// require strict template equality
		ComparePodTemplate(spec.PodTemplate, pod),
		// require pvc compatibility
		comparePersistentVolumeClaims(pod.Spec.Volumes, spec.NodeSpec.VolumeClaimTemplates, state),
		// require strict config equality
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
//
// Since the hash was computed from the existing pod template, before its creation, it only accounts
// for fields in the pod that were set by the operator.
// Any defaulted environment variables, resources, containers from Kubernetes or a mutating webhook is ignored.
// Any label or annotation set by something external (user, webhook, defaulted value) is also ignored.
func ComparePodTemplate(template corev1.PodTemplateSpec, existingPod corev1.Pod) Comparison {
	existingPodHash := hash.GetTemplateHashLabel(existingPod.Labels)
	if existingPodHash == "" {
		return ComparisonMismatch(fmt.Sprintf("No %s label set on the existing pod", hash.TemplateHashLabelName))
	}
	if hash.HashObject(template) != existingPodHash {
		return ComparisonMismatch("Spec hash and running pod spec hash are not equal")
	}
	return ComparisonMatch
}
