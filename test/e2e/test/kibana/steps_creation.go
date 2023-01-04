// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"fmt"

	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func (b Builder) CreationTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Creating Kibana should succeed",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdate(b.RuntimeObjects()...)
			}),
		},
		{
			Name: "Kibana should be created",
			Test: test.Eventually(func() error {
				var createdKb kbv1.Kibana
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Kibana), &createdKb); err != nil {
					return err
				}
				if b.Kibana.Spec.Version != createdKb.Spec.Version {
					return fmt.Errorf("expected version %s but got %s", b.Kibana.Spec.Version, createdKb.Spec.Version)
				}
				// TODO this is incomplete
				return nil
			}),
		},
	}
}
