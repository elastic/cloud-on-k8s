// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build es e2e

package es

import (
	"context"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestKillOneDataNode(t *testing.T) {
	// 1 master + 2 data nodes
	b := elasticsearch.NewBuilder("test-failure-kill-a-data-node").
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithESDataNodes(2, elasticsearch.DefaultResources)

	matchDataNode := func(p corev1.Pod) bool {
		return label.IsDataNode(p) && !label.IsMasterNode(p)
	}

	test.RunRecoverableFailureScenario(t,
		test.KillNodeSteps(matchDataNode, test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name)...),
		b)
}

func TestKillOneMasterNode(t *testing.T) {
	// 2 master + 2 data nodes
	b := elasticsearch.NewBuilder("test-failure-kill-a-master-node").
		WithESMasterNodes(2, elasticsearch.DefaultResources).
		WithESDataNodes(2, elasticsearch.DefaultResources)

	matchMasterNode := func(p corev1.Pod) bool {
		return !label.IsDataNode(p) && label.IsMasterNode(p)
	}

	test.RunRecoverableFailureScenario(t,
		test.KillNodeSteps(matchMasterNode, test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name)...),
		b)
}

func TestKillSingleNodeReusePV(t *testing.T) {
	b := elasticsearch.NewBuilder("test-failure-pvc").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	matchNode := func(p corev1.Pod) bool {
		return true // match first node we find
	}

	test.RunRecoverableFailureScenario(t,
		test.KillNodeSteps(matchNode, test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name)...),
		b)
}

func TestDeleteServices(t *testing.T) {
	b := elasticsearch.NewBuilder("test-failure-delete-services").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	test.Sequence(nil, func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Delete external service",
				Test: func(t *testing.T) {
					s, err := k.GetService(b.Elasticsearch.Namespace, esv1.HTTPService(b.Elasticsearch.Name))
					require.NoError(t, err)
					err = k.Client.Delete(context.Background(), s)
					require.NoError(t, err)
				},
			},
			{
				Name: "Service should be recreated",
				Test: test.Eventually(func() error {
					_, err := k.GetService(b.Elasticsearch.Namespace, esv1.HTTPService(b.Elasticsearch.Name))
					return err
				}),
			},
			// We do not do more checks here, and, particularly, we don't check that the Endpoints resource
			// gets (re)created. There seems to be a bug/race condition in K8s/GKE that occasionally delays Endpoints
			// resource creation when services are quickly created/deleted/created, leading to a flaky test.
			// More details in https://github.com/elastic/cloud-on-k8s/issues/2602.
		}
	}, b).RunSequential(t)
}

func TestDeleteElasticUserSecret(t *testing.T) {
	b := elasticsearch.NewBuilder("test-delete-elastic-user-secret").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	test.RunRecoverableFailureScenario(t, func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Delete elastic user secret",
				Test: func(t *testing.T) {
					key := types.NamespacedName{
						Namespace: test.Ctx().ManagedNamespace(0),
						Name:      b.Elasticsearch.Name + "-es-elastic-user",
					}
					var secret corev1.Secret
					err := k.Client.Get(context.Background(), key, &secret)
					require.NoError(t, err)
					err = k.Client.Delete(context.Background(), &secret)
					require.NoError(t, err)
				},
			},
		}
	}, b)
}

func TestDeleteCACert(t *testing.T) {
	b := elasticsearch.NewBuilder("test-failure-delete-ca-cert").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	test.RunRecoverableFailureScenario(t, func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Delete CA cert",
				Test: func(t *testing.T) {
					key := types.NamespacedName{
						Namespace: test.Ctx().ManagedNamespace(0),
						Name:      b.Elasticsearch.Name + "-es-transport-ca-internal", // ~that's the CA cert secret name \o/~ ... oops not anymore
					}
					var secret corev1.Secret
					err := k.Client.Get(context.Background(), key, &secret)
					require.NoError(t, err)
					err = k.Client.Delete(context.Background(), &secret)
					require.NoError(t, err)
				},
			},
		}
	}, b)
}
