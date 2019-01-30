package stack

import (
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/test/e2e/helpers"
	"github.com/stretchr/testify/require"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke
)

// CreationTestSteps tests the creation of the given stack.
// The stack is not deleted at the end.
func CreationTestSteps(stack v1alpha1.Stack, k *helpers.K8sHelper) helpers.TestStepList {
	return helpers.TestStepList{}.
		WithSteps(
			helpers.TestStep{
				Name: "Creating a stack should succeed",
				Test: func(t *testing.T) {
					err := k.Client.Create(&stack)
					require.NoError(t, err)
				},
			},
			helpers.TestStep{
				Name: "Stack should be created",
				Test: func(t *testing.T) {
					var createdStack v1alpha1.Stack
					err := k.Client.Get(GetNamespacedName(stack), &createdStack)
					require.NoError(t, err)
					require.Equal(t, stack.Spec.Elasticsearch.Version, createdStack.Spec.Elasticsearch.Version)
				},
			},
		).
		WithSteps(CheckStackSteps(stack, k)...)
}
