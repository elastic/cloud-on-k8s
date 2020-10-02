// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package esconfig

import (
	"testing"

	escv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/esconfig/v1alpha1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWebhook(t *testing.T) {
	esc := escv1alpha1.ElasticsearchConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-webhook",
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Spec: escv1alpha1.ElasticsearchConfigSpec{
			Operations: []escv1alpha1.ElasticsearchConfigOperation{
				{URL: "/test",
					// body invalid json, should error out
					Body: `{`,
				},
			},
		},
	}

	err := test.NewK8sClientOrFatal().Client.Create(&esc)
	require.Error(t, err)
	require.Contains(
		t,
		err.Error(),
		`admission webhook "elastic-esconfig-validation-v1alpha1.k8s.elastic.co" denied the request: elasticsearchconfig.k8s.elastic.co "test-webhook" is invalid`,
	)
}
