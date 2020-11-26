// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"fmt"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke
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
				if err := k.Client.Get(k8s.ExtractNamespacedName(&b.Kibana), &createdKb); err != nil {
					return err
				}
				if b.Kibana.Spec.Version != createdKb.Spec.Version {
					return fmt.Errorf("expected version %s but got %s", b.Kibana.Spec.Version, createdKb.Spec.Version)
				}
				return nil
			}),
		},
	}
}
