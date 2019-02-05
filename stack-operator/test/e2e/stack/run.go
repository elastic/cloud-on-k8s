package stack

import (
	"testing"

	"github.com/elastic/k8s-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/k8s-operators/stack-operator/test/e2e/helpers"
)

// RunCreationMutationDeletionTests does exactly what its names is suggesting :)
// If the stack we mutate to is the same as the original stack, tests should still pass.
func RunCreationMutationDeletionTests(t *testing.T, toCreate v1alpha1.Stack, mutateTo v1alpha1.Stack) {
	k := helpers.NewK8sClientOrFatal()
	helpers.TestStepList{}.
		WithSteps(InitTestSteps(toCreate, k)...).
		WithSteps(CreationTestSteps(toCreate, k)...).
		WithSteps(MutationTestSteps(mutateTo, k)...).
		WithSteps(DeletionTestSteps(mutateTo, k)...).
		RunSequential(t)
}
