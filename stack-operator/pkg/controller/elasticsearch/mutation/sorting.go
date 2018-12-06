package mutation

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	corev1 "k8s.io/api/core/v1"
)

// sortPodsByMasterNodeLastAndCreationTimestampAsc sorts pods in a preferred deletion order: current master node always
// last, remaining pods by oldest first.
func sortPodsByMasterNodeLastAndCreationTimestampAsc(masterNode *corev1.Pod, pods []corev1.Pod) func(i, j int) bool {
	return func(i, j int) bool {
		iPod := pods[i]
		jPod := pods[j]

		// sort the master node last
		if masterNode != nil {
			if iPod.Name == masterNode.Name {
				// i is the master node, so it should be last
				return false
			}
			if jPod.Name == masterNode.Name {
				// j is the master node, so it should be last
				return true
			}
		}
		// if neither is the master node, fall back to sorting by creation timestamp, removing the oldest first.
		return iPod.CreationTimestamp.Before(&jPod.CreationTimestamp)
	}
}

// sortPodsByMasterNodesFirstThenNameAsc sorts pods in a preferred creation order: master nodes first, then by name
// the by name part is used to ensure a stable sort order.
func sortPodsByMasterNodesFirstThenNameAsc(pods []corev1.Pod) func(i, j int) bool {
	return func(i, j int) bool {
		// sort by master nodes last
		iPod := pods[i]
		jPod := pods[j]

		iIsMaster := support.NodeTypesMasterLabelName.HasValue(true, iPod.Labels)
		jIsMaster := support.NodeTypesMasterLabelName.HasValue(true, jPod.Labels)

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
}
