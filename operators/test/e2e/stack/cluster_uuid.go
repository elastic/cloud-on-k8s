package stack

import (
	"fmt"
	"testing"

	"github.com/elastic/k8s-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/k8s-operators/stack-operator/test/e2e/helpers"
	"github.com/stretchr/testify/require"
)

// RetrieveClusterUUIDStep stores the current clusterUUID into the given futureClusterUUID
func RetrieveClusterUUIDStep(stack v1alpha1.Stack, k *helpers.K8sHelper, futureClusterUUID *string) helpers.TestStep {
	return helpers.TestStep{
		Name: "Retrieve cluster UUID for comparison purpose",
		Test: helpers.Eventually(func() error {
			var s v1alpha1.Stack
			err := k.Client.Get(GetNamespacedName(stack), &s)
			if err != nil {
				return err
			}
			clusterUUID := s.Status.Elasticsearch.ClusterUUID
			if clusterUUID == "" {
				return fmt.Errorf("Empty ClusterUUID")
			}
			*futureClusterUUID = clusterUUID
			return nil
		}),
	}
}

// CompareClusterUUIDStep compares the current clusterUUID with previousClusterUUID,
// and fails if they don't match
func CompareClusterUUIDStep(stack v1alpha1.Stack, k *helpers.K8sHelper, previousClusterUUID *string) helpers.TestStep {
	return helpers.TestStep{
		Name: "Cluster UUID should have been preserved",
		Test: func(t *testing.T) {
			var s v1alpha1.Stack
			err := k.Client.Get(GetNamespacedName(stack), &s)
			require.NoError(t, err)
			newClusterUUID := s.Status.Elasticsearch.ClusterUUID
			require.NotEmpty(t, *previousClusterUUID)
			require.Equal(t, *previousClusterUUID, newClusterUUID)
		},
	}
}
