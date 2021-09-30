// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"context"
	"fmt"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke
)

func (b Builder) CreationTestSteps(k *test.K8sClient) test.StepList {
	//nolint:thelper
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
