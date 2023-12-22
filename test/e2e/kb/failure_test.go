// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build kb || e2e

package kb

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/kibana"
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
				Test: test.Eventually(func() error {
					var dep appsv1.Deployment
					err := k.Client.Get(context.Background(), types.NamespacedName{
						Namespace: test.Ctx().ManagedNamespace(0),
						Name:      kbv1.Deployment(kbBuilder.Kibana.Name),
					}, &dep)
					if apierrors.IsNotFound(err) {
						// already deleted
						return nil
					}
					err = k.Client.Delete(context.Background(), &dep)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
					return nil
				}),
			},
		}
	}, esBuilder, kbBuilder)
}
