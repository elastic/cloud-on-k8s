// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	"fmt"

	entv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke
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
				var createdEnt entv1beta1.EnterpriseSearch
				if err := k.Client.Get(k8s.ExtractNamespacedName(&b.EnterpriseSearch), &createdEnt); err != nil {
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
