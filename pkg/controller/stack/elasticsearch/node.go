package elasticsearch

import (
	"fmt"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
)

// NextNodeName forms an Elasticsearch node name. Returning the proposed next
// node name to be used for the Elaticsearch cluster node.
func NextNodeName(stack deploymentsv1alpha1.Stack) string {
	return fmt.Sprint(stack.Name, "-elasticsearch-", stack.Status.Elasticsearch.Additions)
}

// FirstNodName forms an Elasticsearch node name. Returning the current oldest
// node name for the Elaticsearch cluster.
func FirstNodName(stack deploymentsv1alpha1.Stack) string {
	return fmt.Sprint(stack.Name, "-elasticsearch-", stack.Status.Elasticsearch.Deletions)
}
