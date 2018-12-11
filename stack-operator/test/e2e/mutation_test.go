package e2e

import (
	"testing"

	common "github.com/elastic/stack-operators/stack-operator/pkg/apis/common/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/test/e2e/stack"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// TestMdiToDedicatedMutation creates a 1 master + data cluster,
// then mutates it to 1 dedicated master + 1 dedicated data cluster
func TestMutationMdiToDedicated(t *testing.T) {
	// create a 1 md node cluster
	initStack := stack.NewStackBuilder("test-mutation-mdi-to-dedicated").
		WithESMasterDataNodes(1, stack.DefaultResources)

	// mutate to 1 m node + 1 d node
	mutatedStack := initStack.
		WithNoESTopologies().
		WithESDataNodes(1, stack.DefaultResources).
		WithESMasterNodes(1, stack.DefaultResources)

	stack.RunCreationMutationDeletionTests(t, initStack.Stack, mutatedStack.Stack)
}

// TestMutationMoreNodes creates a 1 node cluster,
// then mutates it to a 3 nodes cluster
func TestMutationMoreNodes(t *testing.T) {
	// create a stack with 1 node
	initStack := stack.NewStackBuilder("test-mutation-more-nodes").
		WithESMasterDataNodes(1, stack.DefaultResources)
	// mutate it to 2 nodes
	mutatedStack := initStack.
		WithNoESTopologies().
		WithESMasterDataNodes(2, stack.DefaultResources)

	stack.RunCreationMutationDeletionTests(t, initStack.Stack, mutatedStack.Stack)
}

// TestMutationLessNodes creates a 3 node cluster,
// then mutates it to a 1 node cluster
func TestMutationLessNodes(t *testing.T) {
	// create a stack with 3 node
	initStack := stack.NewStackBuilder("test-mutation-less-nodes").
		WithESMasterDataNodes(3, stack.DefaultResources)
	// mutate it to 1 node
	mutatedStack := initStack.
		WithNoESTopologies().
		WithESMasterDataNodes(1, stack.DefaultResources)

	stack.RunCreationMutationDeletionTests(t, initStack.Stack, mutatedStack.Stack)
}

// TestMutationResizeMemoryUp creates a 1 node cluster,
// then mutates it to a 1 node cluster with more RAM
func TestMutationResizeMemoryUp(t *testing.T) {
	// create a stack with a 1G node
	memory1G := resource.MustParse("1G")
	initStack := stack.NewStackBuilder("test-mutation-resize-memory-up").
		WithESMasterDataNodes(1, common.ResourcesSpec{
			Limits: map[corev1.ResourceName]resource.Quantity{
				"memory": memory1G,
			},
		})
	// mutate it to 1 node with 2G memory
	memory2G := resource.MustParse("2G")
	mutatedStack := initStack.
		WithNoESTopologies().
		WithESMasterDataNodes(1, common.ResourcesSpec{
			Limits: map[corev1.ResourceName]resource.Quantity{
				"memory": memory2G,
			},
		})

	stack.RunCreationMutationDeletionTests(t, initStack.Stack, mutatedStack.Stack)
}

// TestMutationResizeMemoryDown creates a 1 node cluster,
// then mutates it to a 1 node cluster with less RAM
func TestMutationResizeMemoryDown(t *testing.T) {
	// create a stack with a 2G node
	memory2G := resource.MustParse("2G")
	initStack := stack.NewStackBuilder("test-mutation-resize-memory-up").
		WithESMasterDataNodes(1, common.ResourcesSpec{
			Limits: map[corev1.ResourceName]resource.Quantity{
				"memory": memory2G,
			},
		})
	// mutate it to 1 node with 1G memory
	memory1G := resource.MustParse("1G")
	mutatedStack := initStack.
		WithNoESTopologies().
		WithESMasterDataNodes(1, common.ResourcesSpec{
			Limits: map[corev1.ResourceName]resource.Quantity{
				"memory": memory1G,
			},
		})

	stack.RunCreationMutationDeletionTests(t, initStack.Stack, mutatedStack.Stack)
}

func TestMutationVersion540To642(t *testing.T) {
	// create a stack with 1 node in version 5.4.0
	initStack := stack.NewStackBuilder("test-mutation-less-nodes").
		WithESMasterDataNodes(3, stack.DefaultResources).
		WithVersion("5.4.0")
	// mutate it to 1 node in version 6.4.2
	mutatedStack := initStack.WithVersion("6.4.2")
	stack.RunCreationMutationDeletionTests(t, initStack.Stack, mutatedStack.Stack)
}
