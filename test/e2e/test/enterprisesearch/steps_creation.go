// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package enterprisesearch

import (
	"context"
	"fmt"

	entv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func (b Builder) CreationTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Creating Enterprise Search should succeed",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdate(b.RuntimeObjects()...)
			}),
		},
		{
			Name: "Enterprise Search should be created",
			Test: test.Eventually(func() error {
				var createdEnt entv1.EnterpriseSearch
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.EnterpriseSearch), &createdEnt); err != nil {
					return err
				}
				if b.EnterpriseSearch.Spec.Version != createdEnt.Spec.Version {
					return fmt.Errorf("expected version %s but got %s", b.EnterpriseSearch.Spec.Version, createdEnt.Spec.Version)
				}
				return nil
			}),
		},
	}
}
