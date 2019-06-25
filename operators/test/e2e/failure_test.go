// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	esname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	kbname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/common"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/kibana"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
					return fmt.Errorf("pod %s not deleted yet", killedPod.Name)
				}),
			},
		}
	}
}

func TestKillOneDataNode(t *testing.T) {
	// 1 master + 2 data nodes
	es := elasticsearch.NewBuilder("test-failure-kill-one-data-node").
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithESDataNodes(2, elasticsearch.DefaultResources)
	matchDataNode := func(p corev1.Pod) bool {
		return label.IsDataNode(p) && !label.IsMasterNode(p)
	}
	common.RunFailureTest(t,
		killNodeTestSteps(helpers.ESPodListOptions(es.Elasticsearch.Name), matchDataNode),
		es)
}

func TestKillOneMasterNode(t *testing.T) {
	// 2 master + 2 data nodes
	es := elasticsearch.NewBuilder("test-failure-kill-one-master-node").
		WithESMasterNodes(2, elasticsearch.DefaultResources).
		WithESDataNodes(2, elasticsearch.DefaultResources)
	matchMasterNode := func(p corev1.Pod) bool {
		return !label.IsDataNode(p) && label.IsMasterNode(p)
	}
	common.RunFailureTest(t,
		killNodeTestSteps(helpers.ESPodListOptions(es.Elasticsearch.Name), matchMasterNode),
		es)
}

func TestKillSingleNodeReusePV(t *testing.T) {
	// TODO :)
	// This test cannot work until we correctly reuse PV between pods in the operator.
	// We should not loose data, and ClusterUUID should stay the same

	// s := elasticsearch.NewBuilder("test-failure-kill-single-node-no-pv").
	// 	WithESMasterDataNodes(1, elasticsearch.DefaultResources).
	//  WithPV().
	// 	Stack
	// matchNode := func(p corev1.Pod) bool {
	// 	return true // match first node we find
	// }
	// killNodeTest(t, s, matchNode)
}

func TestKillKibanaPod(t *testing.T) {
	name := "test-kill-kibana-pod"
	es := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	kb := kibana.NewBuilder(name).
		WithNodeCount(1)

	matchFirst := func(p corev1.Pod) bool {
		return true
	}
	common.RunFailureTest(t,
		killNodeTestSteps(helpers.KibanaPodListOptions(kb.Kibana.Name), matchFirst),
		es, kb)
}

func TestKillKibanaDeployment(t *testing.T) {
	n := "test-kill-kibana-deployment"
	es := elasticsearch.NewBuilder(n).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	kb := kibana.NewBuilder(n).
		WithNodeCount(1)

	common.RunFailureTest(t, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete Kibana deployment",
				Test: func(t *testing.T) {
					var dep appsv1.Deployment
					err := k.Client.Get(types.NamespacedName{
						Namespace: params.Namespace,
						Name:      kbname.Deployment(kb.Kibana.Name),
					}, &dep)
					require.NoError(t, err)
					err = k.Client.Delete(&dep)
					require.NoError(t, err)
				},
			},
		}
	}, es, kb)
}

func TestDeleteServices(t *testing.T) {
	es := elasticsearch.NewBuilder("test-failure-delete-services").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	common.RunFailureTest(t, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete external service",
				Test: func(t *testing.T) {
					s, err := k.GetService(esname.HTTPService(es.Elasticsearch.Name))
					require.NoError(t, err)
					err = k.Client.Delete(s)
					require.NoError(t, err)
				},
			},
		}
	}, es)
}

func TestDeleteElasticUserSecret(t *testing.T) {
	es := elasticsearch.NewBuilder("test-delete-es-elastic-user-secret").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	common.RunFailureTest(t, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete elastic user secret",
				Test: func(t *testing.T) {
					key := types.NamespacedName{
						Namespace: params.Namespace,
						Name:      es.Elasticsearch.Name + "-es-elastic-user",
					}
					var secret corev1.Secret
					err := k.Client.Get(key, &secret)
					require.NoError(t, err)
					err = k.Client.Delete(&secret)
					require.NoError(t, err)
				},
			},
		}
	}, es)
}
func TestDeleteCACert(t *testing.T) {
	es := elasticsearch.NewBuilder("test-failure-delete-ca-cert").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	common.RunFailureTest(t, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete CA cert",
				Test: func(t *testing.T) {
					key := types.NamespacedName{
						Namespace: params.Namespace,
						Name:      es.Elasticsearch.Name + "-es-transport-ca-internal", // ~that's the CA cert secret name \o/~ ... oops not anymore
					}
					var secret corev1.Secret
					err := k.Client.Get(key, &secret)
					require.NoError(t, err)
					err = k.Client.Delete(&secret)
					require.NoError(t, err)
				},
			},
		}
	}, es)
}
