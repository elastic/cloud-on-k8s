package mutation

import (
	"github.com/elastic/k8s-operators/stack-operator/pkg/controller/elasticsearch/label"
	corev1 "k8s.io/api/core/v1"
)

// sortPodsByTerminalFirstMasterNodeLastAndCreationTimestampAsc sorts pods in a preferred deletion order:
// - terminal pods first
// - current master node always last
// - remaining pods by oldest first.
func sortPodsByTerminalFirstMasterNodeLastAndCreationTimestampAsc(
	terminalPods map[string]corev1.Pod,
	masterNode *corev1.Pod,
	pods []corev1.Pod,
) func(i, j int) bool {
	return func(i, j int) bool {
		iPod := pods[i]
		jPod := pods[j]

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

// sortPodByCreationTimestampAsc is a sort function for a list of pods
func sortPodByCreationTimestampAsc(pods []corev1.Pod) func(i, j int) bool {
	return func(i, j int) bool { return pods[i].CreationTimestamp.Before(&pods[j].CreationTimestamp) }
}
