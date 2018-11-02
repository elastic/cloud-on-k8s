package elasticsearch

// stackIDLabelName used to represent a cluster in k8s resources
const stackIDLabelName = "elasticsearch.stack.k8s.elastic.co/id"

func NewLabelsWithStackID(stackID string) map[string]string {
	return map[string]string{stackIDLabelName: stackID}
}
