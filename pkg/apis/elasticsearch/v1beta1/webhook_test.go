// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1beta1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/test"
)

func TestWebhook(t *testing.T) {
	testCases := []test.ValidationWebhookTestCase{
		{
			Name:      "create-valid",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				es := mkElasticsearch(uid)
				return serialize(t, es)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "create-deprecated-version",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				es := mkElasticsearch(uid)
				es.Spec.Version = "7.10.0"
				return serialize(t, es)
			},
			Check: test.ValidationWebhookSucceededWithWarnings(
				`Version 7.10.0 is EOL and support for it will be removed in a future release of the ECK operator`,
			),
		},
		{
			Name:      "create-no-master",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				es := mkElasticsearch(uid)
				es.Spec.NodeSets[0].Count = 0
				return serialize(t, es)
			},
			Check: test.ValidationWebhookFailed(
				`spec.nodeSets: Invalid value`,
			),
		},
		{
			Name:      "update-valid",
			Operation: admissionv1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				es := mkElasticsearch(uid)
				es.Spec.Version = "8.15.0"
				return serialize(t, es)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				es := mkElasticsearch(uid)
				es.Spec.Version = "8.16.0"
				return serialize(t, es)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "update-version-downgrade",
			Operation: admissionv1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				es := mkElasticsearch(uid)
				es.Spec.Version = "8.16.0"
				return serialize(t, es)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				es := mkElasticsearch(uid)
				es.Spec.Version = "8.15.0"
				return serialize(t, es)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "8.15.0": Downgrades are not supported`,
			),
		},
		{
			Name:      "update-deprecated-version",
			Operation: admissionv1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				es := mkElasticsearch(uid)
				es.Spec.Version = "7.10.0"
				return serialize(t, es)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				es := mkElasticsearch(uid)
				es.Spec.Version = "7.10.0"
				return serialize(t, es)
			},
			Check: test.ValidationWebhookSucceededWithWarnings(
				`Version 7.10.0 is EOL and support for it will be removed in a future release of the ECK operator`,
			),
		},
		{
			Name:      "deprecated-version-downgrade-warning-and-denial",
			Operation: admissionv1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				es := mkElasticsearch(uid)
				es.Spec.Version = "7.12.0"
				return serialize(t, es)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				es := mkElasticsearch(uid)
				es.Spec.Version = "7.10.0"
				return serialize(t, es)
			},
			Check: test.ValidationWebhookFailedWithWarnings(
				[]string{`spec.version: Invalid value: "7.10.0": Downgrades are not supported`},
				[]string{`Version 7.10.0 is EOL and support for it will be removed in a future release of the ECK operator`},
			),
		},
	}

	handler := test.NewValidationWebhookHandler(esv1beta1.Validate)
	gvk := metav1.GroupVersionKind{Group: esv1beta1.GroupVersion.Group, Version: esv1beta1.GroupVersion.Version, Kind: "Elasticsearch"}
	test.RunValidationWebhookTests(t, gvk, "elasticsearches", handler, testCases...)
}

func mkElasticsearch(uid string) *esv1beta1.Elasticsearch {
	return &esv1beta1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name: "webhook-test",
			UID:  types.UID(uid),
		},
		Spec: esv1beta1.ElasticsearchSpec{
			Version: "8.16.0",
			NodeSets: []esv1beta1.NodeSet{
				{Name: "default", Count: 1},
			},
		},
	}
}

func serialize(t *testing.T, es *esv1beta1.Elasticsearch) []byte {
	t.Helper()

	objBytes, err := json.Marshal(es)
	require.NoError(t, err)

	return objBytes
}
