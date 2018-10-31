package elasticsearch

// clusterIDLabelName used to represent a cluster in k8s resources
const clusterIDLabelName = "elasticsearch.stack.k8s.elastic.co/cluster-id"

// ClusterID returns the Elasticsearch cluster id
// based on the given namespace and stack name, following
// the convention: <namespace>-<stack name>
func ClusterID(namespace string, stackName string) string {
	return namespace + "-" + stackName
}

// ClusterIDLabels returns a label selector for the given cluster
func ClusterIDLabels(clusterID string) map[string]string {
	return WithClusterIDLabels(map[string]string{}, clusterID)
}

// WithClusterIDLabels returns the given selector augmented with
// the label selector for the given clusterID
func WithClusterIDLabels(labels map[string]string, clusterID string) map[string]string {
	// copy the given labels to keep it unmodified
	newMap := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		newMap[k] = v
	}
	// add clusterID label
	newMap[clusterIDLabelName] = clusterID
	return newMap
}

// ComputeMinimumMasterNodes returns the minimum number of master nodes
// that should be set in a cluster with the given number of nodes
func ComputeMinimumMasterNodes(nodeCount int) int {
	return (nodeCount / 2) + 1
}
