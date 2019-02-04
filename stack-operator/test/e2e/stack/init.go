package stack

import (
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/test/e2e/helpers"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InitTestSteps includes pre-requisite tests (eg. is k8s accessible),
// and cleanup from previous tests
func InitTestSteps(stack v1alpha1.Stack, k *helpers.K8sHelper) []helpers.TestStep {
	return []helpers.TestStep{

		{
			Name: "K8S should be accessible",
			Test: func(t *testing.T) {
				pods := corev1.PodList{}
				err := k.Client.List(&client.ListOptions{}, &pods)
				require.NoError(t, err)
			},
		},

		{
			Name: "Create E2E namespace if needed",
			Test: func(t *testing.T) {
				namespace := corev1.Namespace{
					ObjectMeta: v1.ObjectMeta{
						Name: helpers.DefaultNamespace,
					},
				}
				err := k.Client.Create(&namespace)
				if err != nil && !apierrors.IsAlreadyExists(err) {
					require.NoError(t, err)
				}
			},
		},

		{
			Name: "Stack CRD should exist",
			Test: func(t *testing.T) {
				stacks := v1alpha1.StackList{}
				err := k.Client.List(&client.ListOptions{}, &stacks)
				require.NoError(t, err)
			},
		},

		{
			Name: "Remove the stack if it already exists",
			Test: func(t *testing.T) {
				err := k.Client.Delete(&stack)
				if err != nil {
					// might not exist, which is ok
					require.True(t, apierrors.IsNotFound(err))
				}
				// wait for ES pods to disappear
				helpers.Eventually(func() error {
					return k.CheckPodCount(helpers.ESPodListOptions(stack.Name), 0)
				})(t)
			},
		},
	}
}
