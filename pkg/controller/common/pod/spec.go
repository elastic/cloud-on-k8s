// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pod

import (
	corev1 "k8s.io/api/core/v1"
)

// ContainerByName returns a reference to a container with the name from the given pod spec.
func ContainerByName(podSpec corev1.PodSpec, name string) *corev1.Container {
	for i, c := range podSpec.Containers {
		if c.Name == name {
			return &podSpec.Containers[i]
		}
	}
	return nil
}

func InitContainerByName(podSpec corev1.PodSpec, name string) *corev1.Container {
	for i, c := range podSpec.InitContainers {
		if c.Name == name {
			return &podSpec.InitContainers[i]
		}
	}
	return nil
}
