// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"fmt"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func (b Builder) CreationTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{}.
		WithSteps(test.StepList{
			test.Step{
				Name: "Creating an Elasticsearch cluster should succeed",
				Test: test.Eventually(func() error {
					return k.CreateOrUpdate(b.RuntimeObjects()...)
				}),
			},
			test.Step{
				Name: "Elasticsearch cluster should be created",
				Test: test.Eventually(func() error {
					var createdEs esv1.Elasticsearch
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Elasticsearch), &createdEs); err != nil {
						return err
					}
					if b.Elasticsearch.Spec.Version != createdEs.Spec.Version {
						return fmt.Errorf("expected version %s but got %s", b.Elasticsearch.Spec.Version, createdEs.Spec.Version)
					}
					// TODO this is incomplete
					return nil
				}),
			},
		})
}
