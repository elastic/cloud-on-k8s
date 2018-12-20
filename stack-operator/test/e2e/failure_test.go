package e2e

import (
	"fmt"
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"github.com/elastic/stack-operators/stack-operator/test/e2e/helpers"
	"github.com/elastic/stack-operators/stack-operator/test/e2e/stack"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type failureTestFunc func(k *helpers.K8sHelper) helpers.TestStepList

func RunFailureTest(t *testing.T, s v1alpha1.Stack, f failureTestFunc) {
	k := helpers.NewK8sClientOrFatal()

	var clusterUUID string

	helpers.TestStepList{}.
		WithSteps(stack.InitTestSteps(s, k)...).
		WithSteps(stack.CreationTestSteps(s, k)...).
		WithSteps(stack.RetrieveClusterUUIDStep(s, k, &clusterUUID)).
		// Trigger some kind of catastrophe
		WithSteps(f(k)...).
		// Check we recover
		WithSteps(stack.CheckStackSteps(s, k)...).
		// And that the cluster UUID has not changed
		WithSteps(stack.CompareClusterUUIDStep(s, k, &clusterUUID)).
		WithSteps(stack.DeletionTestSteps(s, k)...).
		RunSequential(t)
}

func killNodeTest(t *testing.T, s v1alpha1.Stack, listOptions client.ListOptions, podMatch func(p corev1.Pod) bool) {
	RunFailureTest(t, s, func(k *helpers.K8sHelper) helpers.TestStepList {
		var killedPod corev1.Pod
		return helpers.TestStepList{
			{
				Name: "Kill a node",
				Test: func(t *testing.T) {
					pods, err := k.GetPods(listOptions)
					require.NoError(t, err)
					var found bool
					killedPod, found = helpers.GetFirstPodMatching(pods, podMatch)
					require.True(t, found)
					err = k.DeletePod(killedPod)
					require.NoError(t, err)
				},
			},
			{
				Name: "Wait for pod to be deleted",
				Test: helpers.Eventually(func() error {
					_, err := k.GetPod(killedPod.Name)
					if apierrors.IsNotFound(err) {
						return nil
					}
					if err != nil {
						return err
					}
					return fmt.Errorf("Pod %s not deleted yet", killedPod.Name)
				}),
			},
		}
	})
}

func TestKillOneDataNode(t *testing.T) {
	// 1 master + 2 data nodes
	s := stack.NewStackBuilder("test-failure-kill-one-data-node").
		WithESMasterNodes(1, stack.DefaultResources).
		WithESDataNodes(2, stack.DefaultResources).
		Stack
	matchDataNode := func(p corev1.Pod) bool {
		return support.IsDataNode(p) && !support.IsMasterNode(p)
	}
	killNodeTest(t, s, helpers.ESPodListOptions(s.Name), matchDataNode)
}

func TestKillOneMasterNode(t *testing.T) {
	// 2 master + 2 data nodes
	s := stack.NewStackBuilder("test-failure-kill-one-master-node").
		WithESMasterNodes(2, stack.DefaultResources).
		WithESDataNodes(2, stack.DefaultResources).
		Stack
	matchMasterNode := func(p corev1.Pod) bool {
		return !support.IsDataNode(p) && support.IsMasterNode(p)
	}
	killNodeTest(t, s, helpers.ESPodListOptions(s.Name), matchMasterNode)
}

func TestKillSingleNodeReusePV(t *testing.T) {
	// TODO :)
	// This test cannot work until we correctly reuse PV between pods in the operator.
	// We should not loose data, and ClusterUUID should stay the same

	// s := stack.NewStackBuilder("test-failure-kill-single-node-no-pv").
	// 	WithESMasterDataNodes(1, stack.DefaultResources).
	//  WithPV().
	// 	Stack
	// matchNode := func(p corev1.Pod) bool {
	// 	return true // match first node we find
	// }
	// killNodeTest(t, s, matchNode)
}

func TestKillKibanaPod(t *testing.T) {
	s := stack.NewStackBuilder("test-kill-kibana-pod").
		WithESMasterDataNodes(1, stack.DefaultResources).
		WithKibana(1).
		Stack
	matchFirst := func(p corev1.Pod) bool {
		return true
	}
	killNodeTest(t, s, helpers.KibanaPodListOptions(s.Name), matchFirst)
}

func TestKillKibanaDeployment(t *testing.T) {
	s := stack.NewStackBuilder("test-kill-kibana-deployment").
		WithESMasterDataNodes(1, stack.DefaultResources).
		WithKibana(1).
		Stack
	RunFailureTest(t, s, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete Kibana deployment",
				Test: func(t *testing.T) {
					var dep appsv1.Deployment
					err := k.Client.Get(helpers.DefaultCtx, types.NamespacedName{
						Namespace: helpers.DefaultNamespace,
						Name:      s.Name + "-kibana",
					}, &dep)
					require.NoError(t, err)
					err = k.Client.Delete(helpers.DefaultCtx, &dep)
					require.NoError(t, err)
				},
			},
		}
	})
}

func TestDeleteServices(t *testing.T) {
	s := stack.NewStackBuilder("test-failure-delete-services").
		WithESMasterDataNodes(1, stack.DefaultResources).
		Stack
	RunFailureTest(t, s, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete discovery service",
				Test: func(t *testing.T) {
					s, err := k.GetService(s.Name + "-es-discovery")
					require.NoError(t, err)
					err = k.Client.Delete(helpers.DefaultCtx, s)
					require.NoError(t, err)
				},
			},
			{
				Name: "Delete public service",
				Test: func(t *testing.T) {
					s, err := k.GetService(s.Name + "-es-public")
					require.NoError(t, err)
					err = k.Client.Delete(helpers.DefaultCtx, s)
					require.NoError(t, err)
				},
			},
		}
	})
}

func TestDeleteElasticUserSecret(t *testing.T) {
	s := stack.NewStackBuilder("test-failure-delete-elastic-user-secret").
		WithESMasterDataNodes(1, stack.DefaultResources).
		Stack
	RunFailureTest(t, s, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete elastic user secret",
				Test: func(t *testing.T) {
					key := types.NamespacedName{
						Namespace: helpers.DefaultNamespace,
						Name:      s.Name + "-elastic-user",
					}
					var secret corev1.Secret
					err := k.Client.Get(helpers.DefaultCtx, key, &secret)
					require.NoError(t, err)
					err = k.Client.Delete(helpers.DefaultCtx, &secret)
					require.NoError(t, err)
				},
			},
		}
	})
}
func TestDeleteCACert(t *testing.T) {
	s := stack.NewStackBuilder("test-failure-delete-ca-cert").
		WithESMasterDataNodes(1, stack.DefaultResources).
		Stack
	RunFailureTest(t, s, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete CA cert",
				Test: func(t *testing.T) {
					key := types.NamespacedName{
						Namespace: helpers.DefaultNamespace,
						Name:      s.Name, // that's the CA cert secret name \o/
					}
					var secret corev1.Secret
					err := k.Client.Get(helpers.DefaultCtx, key, &secret)
					require.NoError(t, err)
					err = k.Client.Delete(helpers.DefaultCtx, &secret)
					require.NoError(t, err)
				},
			},
		}
	})
}
