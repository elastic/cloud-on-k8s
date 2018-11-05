package elasticsearch

import (
	"strings"

	"k8s.io/apimachinery/pkg/util/rand"
)

const (
	// typeSuffix represents the Elasticsearch shortened suffix that is
	// interpolated in NewNodeName.
	typeSuffix = "-es"
	// randomSuffixLength represents the length of the random suffix that is
	// appended in NewNodeName.
	randomSuffixLength = 10
	// k8s object name has a maximum length of 63, we're substracting the
	// randomSuffix and the interpolated type suffix +1 which accounts
	// for the extra `-` in the interpolation.
	maxPrefixLength = 63 - randomSuffixLength - 1 - len(typeSuffix)
)

var (
	// TypeFilter represents the Elasticsearch type filter that is present in a
	// Pod's labels.
	TypeFilter = map[string]string{TypeLabelName: "elasticsearch"}
)

// NewNodeName forms an Elasticsearch node name. Returning a unique node
// node name to be used for the Elaticsearch cluster node.
func NewNodeName(clusterName string) string {
	var prefix = clusterName
	if len(prefix) > maxPrefixLength {
		prefix = prefix[:maxPrefixLength]
	}
	var nodeName strings.Builder
	nodeName.WriteString(prefix)
	nodeName.WriteString(typeSuffix)
	nodeName.WriteString("-")
	nodeName.WriteString(rand.String(randomSuffixLength))
	return nodeName.String()
}
