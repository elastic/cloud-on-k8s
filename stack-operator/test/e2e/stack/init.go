package stack

import (
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/test/e2e/helpers"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InitTestCases includes pre-requisite tests (eg. is k8s accessible),
// and cleanup from previous tests
func InitTestCases(stack v1alpha1.Stack, k *helpers.K8sHelper) []helpers.TestCase {
	return []helpers.TestCase{

		{
			Name: "K8S should be accessible",
			Test: func(t *testing.T) {
				pods := corev1.PodList{}
				err := k.Client.List(helpers.DefaultCtx, &client.ListOptions{}, &pods)
				assert.NoError(t, err)
			},
		},

		{
			Name: "Stack CRD should exist",
			Test: func(t *testing.T) {
				stacks := v1alpha1.StackList{}
				err := k.Client.List(helpers.DefaultCtx, &client.ListOptions{}, &stacks)
				assert.NoError(t, err)
			},
		},

		{
			Name: "Remove the stack if it already exists",
			Test: func(t *testing.T) {
				err := k.Client.Delete(helpers.DefaultCtx, &stack)
				if err != nil {
					// might not exist, which is ok
					assert.True(t, apierrors.IsNotFound(err))
				}
				// wait for ES pods to disappear
				helpers.Eventually(func() error {
					return k.CheckPodCount(helpers.ESPodListOptions(stack.Name), 0)
				})(t)
			},
		},
	}
}
