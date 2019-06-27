// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mutation

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	corev1 "k8s.io/api/core/v1"
)

// sortPodsByTerminalFirstMasterNodeLastAndCreationTimestampAsc sorts pods in a preferred deletion order:
// - terminal pods first
// - current master node always last
// - remaining pods by oldest first.
func sortPodsByTerminalFirstMasterNodeLastAndCreationTimestampAsc(
	terminalPods map[string]corev1.Pod,
	masterNode *corev1.Pod,
	pods PodsToDelete,
) func(i, j int) bool {
	return func(i, j int) bool {
		iPod := pods[i].Pod
		jPod := pods[j].Pod

		_, iIsTerminal := terminalPods[iPod.Name]
		_, jIsTerminal := terminalPods[jPod.Name]

		switch {
		case iIsTerminal && !jIsTerminal:
			return true
		case !iIsTerminal && jIsTerminal:
			return false
		case masterNode != nil && iPod.Name == masterNode.Name:
			return false
		case masterNode != nil && jPod.Name == masterNode.Name:
			return true
		default:
			// if neither is the master node, fall back to sorting by creation timestamp, removing the oldest first.
			return iPod.CreationTimestamp.Before(&jPod.CreationTimestamp)
		}
	}
}

func comparePodByMasterNodesFirstThenNameAsc(iPod corev1.Pod, jPod corev1.Pod) bool {
	iIsMaster := label.NodeTypesMasterLabelName.HasValue(true, iPod.Labels)
	jIsMaster := label.NodeTypesMasterLabelName.HasValue(true, jPod.Labels)

	switch {
	case iIsMaster && !jIsMaster:
		// i is master, j is not, so i should come first
		return true
	case jIsMaster && !iIsMaster:
		// i is not master, j is master, so j should come first
		return false
	default:
		// neither or both are masters, sort by names
		return iPod.Name < jPod.Name
	}
}

// sortPodsToCreateByMasterNodesFirstThenNameAsc sorts podToCreate in a preferred creation order:
// - master nodes first
// - by name otherwise, which is used to ensure a stable sort order.
func sortPodsToCreateByMasterNodesFirstThenNameAsc(podsToCreate []PodToCreate) func(i, j int) bool {
	return func(i, j int) bool {
		return comparePodByMasterNodesFirstThenNameAsc(podsToCreate[i].Pod, podsToCreate[j].Pod)
	}
}

func creationTimestampIsBefore(podi, podj corev1.Pod) bool {
	return podi.CreationTimestamp.Before(&podj.CreationTimestamp)
}

// sortPodByCreationTimestampAsc is a sort function for a list of pods
func sortPodByCreationTimestampAsc(pods pod.PodsWithConfig) func(i, j int) bool {
	return func(i, j int) bool { return creationTimestampIsBefore(pods[i].Pod, pods[j].Pod) }
}

// sortPodtoDeleteByCreationTimestampAsc is a sort function for a list of pods
func sortPodtoDeleteByCreationTimestampAsc(pods PodsToDelete) func(i, j int) bool {
	return func(i, j int) bool { return creationTimestampIsBefore(pods[i].Pod, pods[j].Pod) }
}
