package nodecerts

import (
	"fmt"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TrustRestrictionsFilename is the file name used for the Elasticsearch trust restrictions configuration file.
const TrustRestrictionsFilename = "trust.yml"

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

// Include appends the provided Trust to the current trust config.
func (t *TrustRootConfig) Include(tr v1alpha1.TrustRestrictions) {
	for _, subjectName := range tr.Trust.SubjectName {
		t.Trust.SubjectName = append(t.Trust.SubjectName, subjectName)
	}
}

// LoadTrustRelationships loads the trust relationships from the API.
func LoadTrustRelationships(c k8s.Client, clusterName, namespace string) ([]v1alpha1.TrustRelationship, error) {
	var trustRelationships v1alpha1.TrustRelationshipList
	if err := c.List(&client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{label.ClusterNameLabelName: clusterName}),
		Namespace:     namespace,
	}, &trustRelationships); err != nil {
		return nil, err
	}

	log.Info("Loaded trust relationships", "clusterName", clusterName, "count", len(trustRelationships.Items))

	return trustRelationships.Items, nil
}
