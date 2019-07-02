// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kb

import (
	"testing"

	kbname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/kibana"
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
		WithNodeCount(1)

	matchFirst := func(p corev1.Pod) bool {
		return true
	}
	test.RunFailure(t,
		test.KillNodeSteps(test.KibanaPodListOptions(kbBuilder.Kibana.Name), matchFirst),
		esBuilder, kbBuilder)
}

func TestKillKibanaDeployment(t *testing.T) {
	name := "test-kill-kb-deploy"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	kbBuilder := kibana.NewBuilder(name).
		WithNodeCount(1)

	test.RunFailure(t, func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Delete Kibana deployment",
				Test: func(t *testing.T) {
					var dep appsv1.Deployment
					err := k.Client.Get(types.NamespacedName{
						Namespace: test.Namespace,
						Name:      kbname.Deployment(kbBuilder.Kibana.Name),
					}, &dep)
					require.NoError(t, err)
					err = k.Client.Delete(&dep)
					require.NoError(t, err)
				},
			},
		}
	}, esBuilder, kbBuilder)
}
