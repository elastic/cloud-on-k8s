// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"context"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

func (b Builder) MutationTestSteps(k *test.K8sClient) test.StepList {
	return test.AnnotatePodsWithBuilderHash(b, b.MutatedFrom, k).
		WithSteps(b.UpgradeTestSteps(k)).
		WithSteps(b.CheckK8sTestSteps(k)).
		WithSteps(b.CheckStackTestSteps(k))
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
