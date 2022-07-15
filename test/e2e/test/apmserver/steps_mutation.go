// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"context"

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/generation"
)

func (b Builder) MutationTestSteps(k *test.K8sClient) test.StepList {
	var apmServerGenerationBeforeMutation, apmServerObservedGenerationBeforeMutation int64
	isMutated := b.MutatedFrom != nil

	return test.StepList{
		generation.RetrieveGenerationsStep(&b.ApmServer, k, &apmServerGenerationBeforeMutation, &apmServerObservedGenerationBeforeMutation),
	}.WithSteps(b.UpgradeTestSteps(k)).
		WithSteps(b.CheckK8sTestSteps(k)).
		WithSteps(b.CheckStackTestSteps(k)).
		WithStep(generation.CompareObjectGenerationsStep(&b.ApmServer, k, isMutated, apmServerGenerationBeforeMutation, apmServerObservedGenerationBeforeMutation))
}

func (b Builder) UpgradeTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Applying the ApmServer mutation should succeed",
			Test: test.Eventually(func() error {
				var as apmv1.ApmServer
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.ApmServer), &as); err != nil {
					return err
				}
				as.Spec = b.ApmServer.Spec
				return k.Client.Update(context.Background(), &as)
			}),
		}}
}
