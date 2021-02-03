// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package chaos

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
)

type operatorState struct {
	pods   []corev1.Pod
	leader string
}

func newOperatorState(pods []corev1.Pod, elected string) operatorState {
	// Sort by name to have a stable comparison
	sort.SliceStable(pods, func(i, j int) bool { return pods[i].Name < pods[j].Name })
	return operatorState{
		pods:   pods,
		leader: elected,
	}
}

func (os operatorState) equal(other operatorState) bool {
	// Compare the name of the Pods
	if len(os.pods) != len(other.pods) {
		return false
	}
	for i, pod := range os.pods {
		if pod.Name != other.pods[i].Name {
			return false
		}
	}
	// Compare current leader
	return os.leader == other.leader
}
