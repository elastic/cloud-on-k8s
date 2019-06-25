// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/stretchr/testify/require"
)

func (b Builder) MutationTestSteps(k *helpers.K8sHelper) helpers.TestStepList {
	return helpers.TestStepList{}.
		WithSteps(helpers.TestStepList{
			helpers.TestStep{
				Name: "Applying the Kibana mutation should succeed",
				Test: func(t *testing.T) {

					var curKb v1alpha1.Kibana
					require.NoError(t, k.Client.Get(k8s.ExtractNamespacedName(&b.Kibana), &curKb))
					curKb.Spec = b.Kibana.Spec
					require.NoError(t, k.Client.Update(&curKb))

				},
			},
		}).
		WithSteps(b.CheckStackSteps(k))
}
