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
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/common"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/elasticsearch"
	es "github.com/elastic/cloud-on-k8s/operators/test/e2e/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	kb "github.com/elastic/cloud-on-k8s/operators/test/e2e/kibana"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func RunFailureTest(t *testing.T, s es.Builder, f common.FailureTestFunc) {
	k := helpers.NewK8sClientOrFatal()

	var clusterUUID string

	helpers.TestStepList{}.
		WithSteps(es.InitTestSteps(s, k)...).
		WithSteps(es.CreationTestSteps(s, k)...).
		WithSteps(elasticsearch.RetrieveClusterUUIDStep(s.Elasticsearch, k, &clusterUUID)).
		// Trigger some kind of catastrophe
		WithSteps(f(k)...).
		// Check we recover
		WithSteps(es.CheckStackSteps(s, k)...).
		// And that the cluster UUID has not changed
		WithSteps(elasticsearch.CompareClusterUUIDStep(s.Elasticsearch, k, &clusterUUID)).
		WithSteps(es.DeletionTestSteps(s, k)...).
		RunSequential(t)
}

func killNodeTestSteps(listOptions client.ListOptions, podMatch func(p corev1.Pod) bool) common.FailureTestFunc {
	var killedPod corev1.Pod
	return func(k *helpers.K8sHelper) helpers.TestStepList {
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
					pod, err := k.GetPod(killedPod.Name)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
					if apierrors.IsNotFound(err) || killedPod.UID != pod.UID {
						return nil
					}
					return fmt.Errorf("Pod %s not deleted yet", killedPod.Name)
				}),
			},
		}
	}
}

func TestKillOneDataNode(t *testing.T) {
	// 1 master + 2 data nodes
	s := es.NewBuilder("test-failure-kill-one-data-node").
		WithESMasterNodes(1, es.DefaultResources).
		WithESDataNodes(2, es.DefaultResources)
	matchDataNode := func(p corev1.Pod) bool {
		return label.IsDataNode(p) && !label.IsMasterNode(p)
	}
	RunFailureTest(t, s,
		killNodeTestSteps(helpers.ESPodListOptions(s.Elasticsearch.Name), matchDataNode),
	)
}

func TestKillOneMasterNode(t *testing.T) {
	// 2 master + 2 data nodes
	s := es.NewBuilder("test-failure-kill-one-master-node").
		WithESMasterNodes(2, es.DefaultResources).
		WithESDataNodes(2, es.DefaultResources)
	matchMasterNode := func(p corev1.Pod) bool {
		return !label.IsDataNode(p) && label.IsMasterNode(p)
	}
	RunFailureTest(t, s,
		killNodeTestSteps(helpers.ESPodListOptions(s.Elasticsearch.Name), matchMasterNode),
	)
}

func TestKillSingleNodeReusePV(t *testing.T) {
	// TODO :)
	// This test cannot work until we correctly reuse PV between pods in the operator.
	// We should not loose data, and ClusterUUID should stay the same

	// s := es.NewBuilder("test-failure-kill-single-node-no-pv").
	// 	WithESMasterDataNodes(1, es.DefaultResources).
	//  WithPV().
	// 	Stack
	// matchNode := func(p corev1.Pod) bool {
	// 	return true // match first node we find
	// }
	// killNodeTest(t, s, matchNode)
}

func TestKillKibanaPod(t *testing.T) {
	s := kb.NewBuilder("test-kill-kibana-pod").
		WithKibana(1)
	matchFirst := func(p corev1.Pod) bool {
		return true
	}
	kb.RunFailureTest(t, s,
		killNodeTestSteps(helpers.KibanaPodListOptions(s.Kibana.Name), matchFirst),
	)
}

func TestKillKibanaDeployment(t *testing.T) {
	b := kb.NewBuilder("test-kill-kibana-deployment").
		WithKibana(1)
	kb.RunFailureTest(t, b, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete Kibana deployment",
				Test: func(t *testing.T) {
					var dep appsv1.Deployment
					err := k.Client.Get(types.NamespacedName{
						Namespace: params.Namespace,
						Name:      kbname.Deployment(b.Kibana.Name),
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
	s := es.NewBuilder("test-failure-delete-services").
		WithESMasterDataNodes(1, es.DefaultResources)
	RunFailureTest(t, s, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
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
	s := es.NewBuilder("test-delete-es-elastic-user-secret").
		WithESMasterDataNodes(1, es.DefaultResources)
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
	s := es.NewBuilder("test-failure-delete-ca-cert").
		WithESMasterDataNodes(1, es.DefaultResources)
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
