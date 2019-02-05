package operator

import "github.com/elastic/k8s-operators/operators/pkg/utils/net"

// Parameters contain parameters to create new operators.
type Parameters struct {
	// OperatorImage is the operator docker image. The operator needs to be aware of its image to use it in sidecars.
	OperatorImage string
	// Dialer is used to create the Elasticsearch HTTP client.
	Dialer net.Dialer
}
