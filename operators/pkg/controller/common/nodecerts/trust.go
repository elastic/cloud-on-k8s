package nodecerts

import (
	"fmt"
)

// TrustRootConfig is the root of an Elasticsearch trust restrictions file
type TrustRootConfig struct {
	// Trust contains configuration for the Elasticsearch trust restrictions
	Trust TrustConfig `json:"trust,omitempty"`
}

// TrustConfig contains configuration for the Elasticsearch trust restrictions
type TrustConfig struct {
	// SubjectName is a list of patterns that incoming TLS client certificates must match
	SubjectName []string `json:"subject_name,omitempty"`
}

// NewTrustRootConfig returns a TrustRootConfig configured with the given
// cluster name and namespace as subject name
func NewTrustRootConfig(clusterName string, namespace string) TrustRootConfig {
	return TrustRootConfig{
		Trust: TrustConfig{
			// the Subject Name needs to match the certificates of the nodes we want to allow to connect.
			// this needs to be kept in sync with buildCertificateCommonName
			SubjectName: []string{fmt.Sprintf(
				"*.node.%s.%s.es.cluster.local", clusterName, namespace,
			)},
		},
	}
}
