package stack

import (
	"github.com/elastic/k8s-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/k8s-operators/stack-operator/test/e2e/helpers"
)

// CheckStackSteps returns all test steps to verify the status of the given stack
func CheckStackSteps(stack v1alpha1.Stack, k8sClient *helpers.K8sHelper) helpers.TestStepList {
	return helpers.TestStepList{}.
		WithSteps(K8sStackChecks(stack, k8sClient)...).
		WithSteps(ESClusterChecks(stack, k8sClient)...)
}
