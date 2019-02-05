package stack

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	"github.com/stretchr/testify/require"

	"github.com/elastic/k8s-operators/operators/pkg/apis/deployments/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// DeletionTestSteps tests the deletion of the given stack
func DeletionTestSteps(stack v1alpha1.Stack, k *helpers.K8sHelper) []helpers.TestStep {
	return []helpers.TestStep{
		{
			Name: "Deleting stack should return no error",
			Test: func(t *testing.T) {
				err := k.Client.Delete(&stack)
				require.NoError(t, err)
			},
		},
		{
			Name: "Stack should not be there anymore",
			Test: func(t *testing.T) {
				var s v1alpha1.Stack
				err := k.Client.Get(types.NamespacedName{
					Name:      stack.GetName(),
					Namespace: stack.GetNamespace(),
				}, &s)
				require.Error(t, err)
				require.True(t, apierrors.IsNotFound(err))
			},
		},
		{
			Name: "ES pods should be eventually be removed",
			Test: helpers.Eventually(func() error {
				return k.CheckPodCount(helpers.ESPodListOptions(stack.Name), 0)
			}),
		},
	}
}
