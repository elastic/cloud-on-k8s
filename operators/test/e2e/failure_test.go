// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	kbname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/stack"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type failureTestFunc func(k *helpers.K8sHelper) helpers.TestStepList

func RunFailureTest(t *testing.T, s stack.Builder, f failureTestFunc) {
	k := helpers.NewK8sClientOrFatal()

	var clusterUUID string

	helpers.TestStepList{}.
		WithSteps(stack.InitTestSteps(s, k)...).
		WithSteps(stack.CreationTestSteps(s, k)...).
		WithSteps(stack.RetrieveClusterUUIDStep(s.Elasticsearch, k, &clusterUUID)).
		// Trigger some kind of catastrophe
		WithSteps(f(k)...).
		// Check we recover
		WithSteps(stack.CheckStackSteps(s, k)...).
		// And that the cluster UUID has not changed
		WithSteps(stack.CompareClusterUUIDStep(s.Elasticsearch, k, &clusterUUID)).
		WithSteps(stack.DeletionTestSteps(s, k)...).
		RunSequential(t)
}

func killNodeTest(t *testing.T, s stack.Builder, listOptions client.ListOptions, podMatch func(p corev1.Pod) bool) {
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
		WithESDataNodes(2, stack.DefaultResources)
	matchDataNode := func(p corev1.Pod) bool {
		return label.IsDataNode(p) && !label.IsMasterNode(p)
	}
	killNodeTest(t, s, helpers.ESPodListOptions(s.Elasticsearch.Name), matchDataNode)
}

func TestKillOneMasterNode(t *testing.T) {
	// 2 master + 2 data nodes
	s := stack.NewStackBuilder("test-failure-kill-one-master-node").
		WithESMasterNodes(2, stack.DefaultResources).
		WithESDataNodes(2, stack.DefaultResources)
	matchMasterNode := func(p corev1.Pod) bool {
		return !label.IsDataNode(p) && label.IsMasterNode(p)
	}
	killNodeTest(t, s, helpers.ESPodListOptions(s.Elasticsearch.Name), matchMasterNode)
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
		WithKibana(1)
	matchFirst := func(p corev1.Pod) bool {
		return true
	}
	killNodeTest(t, s, helpers.KibanaPodListOptions(s.Kibana.Name), matchFirst)
}

func TestKillKibanaDeployment(t *testing.T) {
	s := stack.NewStackBuilder("test-kill-kibana-deployment").
		WithESMasterDataNodes(1, stack.DefaultResources).
		WithKibana(1)
	RunFailureTest(t, s, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete Kibana deployment",
				Test: func(t *testing.T) {
					var dep appsv1.Deployment
					err := k.Client.Get(types.NamespacedName{
						Namespace: params.Namespace,
						Name:      kbname.Deployment(s.Kibana.Name),
					}, &dep)
					require.NoError(t, err)
					err = k.Client.Delete(&dep)
					require.NoError(t, err)
				},
			},
		}
	})
}

func TestDeleteServices(t *testing.T) {
	s := stack.NewStackBuilder("test-failure-delete-services").
		WithESMasterDataNodes(1, stack.DefaultResources)
	RunFailureTest(t, s, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete discovery service",
				Test: func(t *testing.T) {
					s, err := k.GetService(s.Elasticsearch.Name + "-es-discovery")
					require.NoError(t, err)
					err = k.Client.Delete(s)
					require.NoError(t, err)
				},
			},
			{
				Name: "Delete external service",
				Test: func(t *testing.T) {
					s, err := k.GetService(name.HTTPService(s.Elasticsearch.Name))
					require.NoError(t, err)
					err = k.Client.Delete(s)
					require.NoError(t, err)
				},
			},
		}
	})
}

func TestDeleteElasticUserSecret(t *testing.T) {
	s := stack.NewStackBuilder("test-delete-es-elastic-user-secret").
		WithESMasterDataNodes(1, stack.DefaultResources)
	RunFailureTest(t, s, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete elastic user secret",
				Test: func(t *testing.T) {
					key := types.NamespacedName{
						Namespace: params.Namespace,
						Name:      s.Elasticsearch.Name + "-es-elastic-user",
					}
					var secret corev1.Secret
					err := k.Client.Get(key, &secret)
					require.NoError(t, err)
					err = k.Client.Delete(&secret)
					require.NoError(t, err)
				},
			},
		}
	})
}
func TestDeleteCACert(t *testing.T) {
	s := stack.NewStackBuilder("test-failure-delete-ca-cert").
		WithESMasterDataNodes(1, stack.DefaultResources)
	RunFailureTest(t, s, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete CA cert",
				Test: func(t *testing.T) {
					key := types.NamespacedName{
						Namespace: params.Namespace,
						Name:      s.Elasticsearch.Name + "-es-transport-ca-internal", // ~that's the CA cert secret name \o/~ ... oops not anymore
					}
					var secret corev1.Secret
					err := k.Client.Get(key, &secret)
					require.NoError(t, err)
					err = k.Client.Delete(&secret)
					require.NoError(t, err)
				},
			},
		}
	})
}
