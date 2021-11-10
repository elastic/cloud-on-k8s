// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
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
			test.Step{
				Name: "Give Elasticsearch some time to allocate internal indices",
				Test: func(t *testing.T) {
					// TODO remove this step once https://github.com/elastic/cloud-on-k8s/issues/5040 does not apply anymore
					time.Sleep(30 * time.Second)
				},
				Skip: func() bool {
					return version.MustParse(b.Elasticsearch.Spec.Version).LT(version.MinFor(7, 16, 0))
				},
			},
		})
}
