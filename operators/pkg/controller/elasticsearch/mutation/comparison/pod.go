// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package comparison

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
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

	actualContainer, err := getEsContainer(pod.Spec.Containers)
	if err != nil {
		return false, nil, err
	}
	expectedContainer, err := getEsContainer(spec.PodSpec.Containers)
	if err != nil {
		return false, nil, err
	}

	comparisons := []Comparison{
		NewStringComparison(name.Basename(name.NewPodName(es.Name, spec.NodeSpec)), name.Basename(pod.Name), "Pod base name"),
		NewStringComparison(expectedContainer.Image, actualContainer.Image, "Docker image"),
		NewStringComparison(expectedContainer.Name, actualContainer.Name, "Container name"),
		compareEnvironmentVariables(actualContainer.Env, expectedContainer.Env),
		compareResources(actualContainer.Resources, expectedContainer.Resources),
		comparePersistentVolumeClaims(pod.Spec.Volumes, spec.NodeSpec.VolumeClaimTemplates, state),
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
		if c.Name == v1alpha1.ElasticsearchContainerName {
			return c, nil
		}
	}
	return corev1.Container{}, fmt.Errorf("no container named %s in the given pod", v1alpha1.ElasticsearchContainerName)
}
