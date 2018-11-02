package elasticsearch

// ComputeMinimumMasterNodes returns the minimum number of master nodes
// that should be set in a cluster with the given number of nodes
func ComputeMinimumMasterNodes(nodeCount int) int {
	return (nodeCount / 2) + 1
}
