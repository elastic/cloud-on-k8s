// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/generation"
)

func (b Builder) MutationTestSteps(k *test.K8sClient) test.StepList {
	isMutated := b.MutatedFrom != nil
	var agentGenerationBeforeMutation, agentObservedGenerationBeforeMutation int64
	return test.AnnotatePodsWithBuilderHash(b, b.MutatedFrom, k).
		WithStep(generation.RetrieveGenerationsStep(&b.Kibana, k, &agentGenerationBeforeMutation, &agentObservedGenerationBeforeMutation)).
		WithSteps(b.UpgradeTestSteps(k)).
		WithSteps(b.CheckK8sTestSteps(k)).
		WithSteps(b.CheckStackTestSteps(k)).
		WithStep(generation.CompareObjectGenerationsStep(&b.Kibana, k, isMutated, agentGenerationBeforeMutation, agentObservedGenerationBeforeMutation))
}

func (b Builder) MutationReversalTestContext() test.ReversalTestContext {
	panic("not implemented")
}

func (b Builder) UpgradeTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Applying the Kibana mutation should succeed",
			Test: test.Eventually(func() error {
				var kb kbv1.Kibana
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Kibana), &kb); err != nil {
					return err
				}
				kb.Spec = b.Kibana.Spec
				return k.Client.Update(context.Background(), &kb)
			}),
		}}
}
