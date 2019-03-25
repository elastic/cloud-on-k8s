// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package comparison

import (
	"fmt"
	"strconv"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/reconcile"
	corev1 "k8s.io/api/core/v1"
)

const (
	// TaintedAnnotationName used to represent a tainted resource in k8s resources
	TaintedAnnotationName = "elasticsearch.k8s.elastic.co/tainted"

	// TaintedReason message
	TaintedReason = "mismatch due to tainted node"
)

func PodMatchesSpec(podWithConfig pod.PodWithConfig, spec pod.PodSpecContext, state reconcile.ResourcesState) (bool, []string, error) {
	pod := podWithConfig.Pod
	config := podWithConfig.Config

	actualContainer, err := getEsContainer(pod.Spec.Containers)
	if err != nil {
		return false, nil, err
	}
	expectedContainer, err := getEsContainer(spec.PodSpec.Containers)
	if err != nil {
		return false, nil, err
	}

	comparisons := []Comparison{
		NewStringComparison(expectedContainer.Image, actualContainer.Image, "Docker image"),
		NewStringComparison(expectedContainer.Name, actualContainer.Name, "Container name"),
		compareEnvironmentVariables(actualContainer.Env, expectedContainer.Env),
		compareResources(actualContainer.Resources, expectedContainer.Resources),
		comparePersistentVolumeClaims(pod.Spec.Volumes, spec.TopologyElement.VolumeClaimTemplates, state),
		compareConfigs(config, spec.Config),
		// Non-exhaustive list of ignored stuff:
		// - pod labels
		// - probe password
		// - volume and volume mounts
		// - readiness probe
		// - termination grace period
		// - ports
		// - image pull policy
	}

	for _, c := range comparisons {
		if !c.Match {
			return false, c.MismatchReasons, nil
		}
	}

	return true, nil, nil
}

// getEsContainer returns the elasticsearch container in the given pod
func getEsContainer(containers []corev1.Container) (corev1.Container, error) {
	for _, c := range containers {
		if c.Name == pod.DefaultContainerName {
			return c, nil
		}
	}
	return corev1.Container{}, fmt.Errorf("no container named %s in the given pod", pod.DefaultContainerName)
}

func IsTainted(pod corev1.Pod) bool {
	if v, ok := pod.Annotations[TaintedAnnotationName]; ok {
		tainted, _ := strconv.ParseBool(v) // #nosec G104 // ignore unhandled error because we want to default to false
		return tainted
	}
	return false
}
