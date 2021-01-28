// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build kb e2e

package kb

import (
	"context"
	"testing"

	kibana2 "github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestKillKibanaPod(t *testing.T) {
	name := "test-kill-kb-pod"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	matchFirst := func(p corev1.Pod) bool {
		return true
	}
	test.RunRecoverableFailureScenario(t,
		test.KillNodeSteps(matchFirst, test.KibanaPodListOptions(kbBuilder.Kibana.Namespace, kbBuilder.Kibana.Name)...),
		esBuilder, kbBuilder)
}

func TestKillKibanaDeployment(t *testing.T) {
	name := "test-kill-kb-deploy"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	test.RunRecoverableFailureScenario(t, func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Delete Kibana deployment",
				Test: func(t *testing.T) {
					var dep appsv1.Deployment
					err := k.Client.Get(context.Background(), types.NamespacedName{
						Namespace: test.Ctx().ManagedNamespace(0),
						Name:      kibana2.Deployment(kbBuilder.Kibana.Name),
					}, &dep)
					require.NoError(t, err)
					err = k.Client.Delete(context.Background(), &dep)
					require.NoError(t, err)
				},
			},
		}
	}, esBuilder, kbBuilder)
}
