package kibana

// stackIDLabelName used to represent a Kibana in k8s resources
const stackIDLabelName = "kibana.stack.k8s.elastic.co/id"

func NewLabelsWithStackID(stackID string) map[string]string {
	return map[string]string{stackIDLabelName: stackID}
}
