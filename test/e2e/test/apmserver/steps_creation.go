// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"context"
	"fmt"

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func (b Builder) CreationTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Creating APM Server should succeed",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdate(b.RuntimeObjects()...)
			}),
		},
		{
			Name: "APM Server should be created",
			Test: test.Eventually(func() error {
				var createdApmServer apmv1.ApmServer
				err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.ApmServer), &createdApmServer)
				if err != nil {
					return err
				}
				if b.ApmServer.Spec.Version != createdApmServer.Spec.Version {
					return fmt.Errorf("expected version %s but got %s", b.ApmServer.Spec.Version, createdApmServer.Spec.Version)
				}
				return nil
				// TODO this is incomplete
			}),
		},
	}
}
