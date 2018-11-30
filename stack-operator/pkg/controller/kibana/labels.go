package kibana

// kibanaIdentifierLabelName used to represent a Kibana in k8s resources
const kibanaNameLabelName = "kibana.k8s.elastic.co/name"

func NewLabelsWithKibanaName(kibanaName string) map[string]string {
	return map[string]string{kibanaNameLabelName: kibanaName}
}
