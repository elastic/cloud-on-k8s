package elasticsearch

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/rand"
)

const (
	typeSuffix = "-es"

	randomSuffixLength = 10
	// k8s object name has a maximum length
	maxNameLength = 63 - randomSuffixLength - 1 - 3
)

var (
	// TypeFilter represents the Elasticsearch type filter that is present in a
	// Pod's labels.
	TypeFilter = map[string]string{TypeLabelName: "elasticsearch"}
)

// NewNodeName forms an Elasticsearch node name. Returning a unique node
// node name to be used for the Elaticsearch cluster node.
func NewNodeName(clusterName string) string {
	var prefix = fmt.Sprint(clusterName)
	var suffix = rand.String(randomSuffixLength - (len(typeSuffix)))
	if len(prefix) > maxNameLength {
		prefix = prefix[:maxNameLength]
	}
	return fmt.Sprint(prefix, "-es-", suffix)
}
