// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"context"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

func (b Builder) MutationTestSteps(k *test.K8sClient) test.StepList {
	panic("not implemented")
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
