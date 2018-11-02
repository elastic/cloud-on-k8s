package kibana

// clusterIDLabelName used to represent a cluster in k8s resources
const clusterIDLabelName = "kibana.stack.k8s.elastic.co/cluster-id"

func NewLabelsWithClusterID(clusterID string) map[string]string {
	return map[string]string{clusterIDLabelName: clusterID}
}
