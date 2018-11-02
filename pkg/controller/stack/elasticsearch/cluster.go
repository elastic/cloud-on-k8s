package elasticsearch

// ClusterID returns the Elasticsearch cluster id
// based on the given namespace and stack name, following
// the convention: <namespace>-<stack name>
func ClusterID(namespace string, stackName string) string {
	return namespace + "-" + stackName
}

// ComputeMinimumMasterNodes returns the minimum number of master nodes
// that should be set in a cluster with the given number of nodes
func ComputeMinimumMasterNodes(nodeCount int) int {
	return (nodeCount / 2) + 1
}
