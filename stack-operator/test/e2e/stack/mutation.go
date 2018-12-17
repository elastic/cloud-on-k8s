package stack

import (
	"fmt"
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/test/e2e/helpers"
	"github.com/stretchr/testify/require"
)

// MutationTestSteps tests topology changes on the given stack
// we expect the stack to be already created and running.
// If the stack to mutate to is the same as the original stack,
// then all tests should still pass.
func MutationTestSteps(stack v1alpha1.Stack, k *helpers.K8sHelper) []helpers.TestStep {

	var clusterIDBeforeMutation string

	// TODO: continuous ES requests to check cluster health is always at least yellow

	return helpers.TestStepList{}.
		WithSteps(
			helpers.TestStep{
				Name: "Retrieve cluster ID before mutation for comparison purpose",
				Test: helpers.Eventually(func() error {
					var s v1alpha1.Stack
					err := k.Client.Get(helpers.DefaultCtx, GetNamespacedName(stack), &s)
					if err != nil {
						return err
					}
					clusterIDBeforeMutation = s.Status.Elasticsearch.ClusterUUID
					if clusterIDBeforeMutation == "" {
						return fmt.Errorf("Empty ClusterUUID")
					}
					return nil
				}),
			},
			helpers.TestStep{
				Name: "Applying the mutation should succeed",
				Test: func(t *testing.T) {
					// get stack so we have a versioned k8s resource we can update
					var stackRes v1alpha1.Stack
					err := k.Client.Get(helpers.DefaultCtx, GetNamespacedName(stack), &stackRes)
					require.NoError(t, err)
					// update with new stack spec
					stackRes.Spec = stack.Spec
					err = k.Client.Update(helpers.DefaultCtx, &stackRes)
					require.NoError(t, err)
				},
			}).
		WithSteps(CheckStackSteps(stack, k)...).
		WithSteps(
			helpers.TestStep{
				Name: "Cluster UUID should be preserved after mutation is done",
				Test: func(t *testing.T) {
					var s v1alpha1.Stack
					err := k.Client.Get(helpers.DefaultCtx, GetNamespacedName(stack), &s)
					require.NoError(t, err)
					clusterIDAfterMutation := s.Status.Elasticsearch.ClusterUUID
					require.NotEmpty(t, clusterIDBeforeMutation)
					require.Equal(t, clusterIDBeforeMutation, clusterIDAfterMutation)
				},
			},
		)
}
