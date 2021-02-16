// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"context"
	"testing"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/stretchr/testify/require"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke
)

func (b Builder) CreationTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Creating Kibana should succeed",
			Test: func(t *testing.T) {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Create(context.Background(), obj)
					require.NoError(t, err)
				}
			},
		},
		{
			Name: "Kibana should be created",
			Test: func(t *testing.T) {
				var createdKb kbv1.Kibana
				err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Kibana), &createdKb)
				require.NoError(t, err)
				require.Equal(t, b.Kibana.Spec.Version, createdKb.Spec.Version)
				// TODO this is incomplete
			},
		},
	}
}
