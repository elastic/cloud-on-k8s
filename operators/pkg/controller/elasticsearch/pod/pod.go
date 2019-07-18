// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pod

import (
	corev1 "k8s.io/api/core/v1"
)

// PodMapToNames returns a list of pod names from a map of pod names to pods
func PodMapToNames(pods map[string]corev1.Pod) []string {
	names := make([]string, 0, len(pods))
	for podName := range pods {
		names = append(names, podName)
	}
	return names
}

// PodsByName returns a map of pod names to pods
func PodsByName(pods []corev1.Pod) map[string]corev1.Pod {
	podMap := make(map[string]corev1.Pod, len(pods))
	for _, pod := range pods {
		podMap[pod.Name] = pod
	}
	return podMap
}
