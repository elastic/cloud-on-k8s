// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	esname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework/elasticsearch"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestKillOneDataNode(t *testing.T) {
	// 1 master + 2 data nodes
	b := elasticsearch.NewBuilder("test-failure-kill-one-data-node").
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithESDataNodes(2, elasticsearch.DefaultResources)

	matchDataNode := func(p corev1.Pod) bool {
		return label.IsDataNode(p) && !label.IsMasterNode(p)
	}

	framework.RunFailureTest(t,
		framework.KillNodeTestSteps(framework.ESPodListOptions(b.Elasticsearch.Name), matchDataNode),
		b)
}

func TestKillOneMasterNode(t *testing.T) {
	// 2 master + 2 data nodes
	b := elasticsearch.NewBuilder("test-failure-kill-one-master-node").
		WithESMasterNodes(2, elasticsearch.DefaultResources).
		WithESDataNodes(2, elasticsearch.DefaultResources)

	matchMasterNode := func(p corev1.Pod) bool {
		return !label.IsDataNode(p) && label.IsMasterNode(p)
	}

	framework.RunFailureTest(t,
		framework.KillNodeTestSteps(framework.ESPodListOptions(b.Elasticsearch.Name), matchMasterNode),
		b)
}

func TestKillSingleNodeReusePV(t *testing.T) {
	// TODO :)
	// This test cannot work until we correctly reuse PV between pods in the operator.
	// We should not loose data, and ClusterUUID should stay the same

	// b := elasticsearch.NewBuilder("test-failure-kill-single-node-no-pv").
	// 	WithESMasterDataNodes(1, elasticsearch.DefaultResources).
	//  WithPV().
	// matchNode := func(p corev1.Pod) bool {
	// 	return true // match first node we find
	// }
	// killNodeTest(t, b, matchNode)
}

func TestDeleteServices(t *testing.T) {
	b := elasticsearch.NewBuilder("test-failure-delete-services").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	framework.RunFailureTest(t, func(k *framework.K8sClient) framework.TestStepList {
		return framework.TestStepList{
			{
				Name: "Delete external service",
				Test: func(t *testing.T) {
					s, err := k.GetService(esname.HTTPService(b.Elasticsearch.Name))
					require.NoError(t, err)
					err = k.Client.Delete(s)
					require.NoError(t, err)
				},
			},
		}
	}, b)
}

func TestDeleteElasticUserSecret(t *testing.T) {
	b := elasticsearch.NewBuilder("test-delete-es-elastic-user-secret").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	framework.RunFailureTest(t, func(k *framework.K8sClient) framework.TestStepList {
		return framework.TestStepList{
			{
				Name: "Delete elastic user secret",
				Test: func(t *testing.T) {
					key := types.NamespacedName{
						Namespace: framework.Namespace,
						Name:      b.Elasticsearch.Name + "-es-elastic-user",
					}
					var secret corev1.Secret
					err := k.Client.Get(key, &secret)
					require.NoError(t, err)
					err = k.Client.Delete(&secret)
					require.NoError(t, err)
				},
			},
		}
	}, b)
}
func TestDeleteCACert(t *testing.T) {
	b := elasticsearch.NewBuilder("test-failure-delete-ca-cert").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	framework.RunFailureTest(t, func(k *framework.K8sClient) framework.TestStepList {
		return framework.TestStepList{
			{
				Name: "Delete CA cert",
				Test: func(t *testing.T) {
					key := types.NamespacedName{
						Namespace: framework.Namespace,
						Name:      b.Elasticsearch.Name + "-es-transport-ca-internal", // ~that's the CA cert secret name \o/~ ... oops not anymore
					}
					var secret corev1.Secret
					err := k.Client.Get(key, &secret)
					require.NoError(t, err)
					err = k.Client.Delete(&secret)
					require.NoError(t, err)
				},
			},
		}
	}, b)
}
