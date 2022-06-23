// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/test"
)

func TestWebhook(t *testing.T) {
	testCases := []test.ValidationWebhookTestCase{
		{
			Name:      "create-valid",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "unknown-field",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				ent.SetAnnotations(map[string]string{
					corev1.LastAppliedConfigAnnotation: `{"metadata":{"name": "ekesn", "namespace": "default", "uid": "e7a18cfb-b017-475c-8da2-1ec941b1f285", "creationTimestamp":"2020-03-24T13:43:20Z" },"spec":{"version":"7.6.1", "unknown": "UNKNOWN"}}`,
				})
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookFailed(
				`"unknown": unknown field found in the kubectl.kubernetes.io/last-applied-configuration annotation is unknown`,
			),
		},
		{
			Name:      "long-name",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				ent.SetName(strings.Repeat("x", 100))
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookFailed(
				`metadata.name: Too long: must have at most 36 bytes`,
			),
		},
		{
			Name:      "invalid-version",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				ent.Spec.Version = "7.x"
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "7.x": Invalid version: No Major.Minor.Patch elements found`,
			),
		},
		{
			Name:      "unsupported-version-lower",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				ent.Spec.Version = "3.1.2"
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "3.1.2": Unsupported version: version 3.1.2 is lower than the lowest supported version`,
			),
		},
		{
			Name:      "unsupported-version-higher",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				ent.Spec.Version = "300.1.2"
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "300.1.2": Unsupported version: version 300.1.2 is higher than the highest supported version`,
			),
		},
		{
			Name:      "update-valid",
			Operation: admissionv1beta1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				ent.Spec.Version = "7.7.0"
				return serialize(t, ent)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				ent.Spec.Version = "7.7.1"
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "version-downgrade",
			Operation: admissionv1beta1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				ent.Spec.Version = "7.7.1"
				return serialize(t, ent)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				ent.Spec.Version = "7.7.0"
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Forbidden: Version downgrades are not supported`,
			),
		},
		{
			Name:      "version-downgrade-with-override",
			Operation: admissionv1beta1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				ent.Spec.Version = "7.7.1"
				return serialize(t, ent)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				ent.Spec.Version = "7.7.0"
				ent.Annotations = map[string]string{
					commonv1.DisableDowngradeValidationAnnotation: "true",
				}
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "named-es-ref",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				ent.Spec.ElasticsearchRef = commonv1.ObjectSelector{Name: "esname", Namespace: "esns", ServiceName: "essvc"}
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "secret-named-es-ref",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				ent.Spec.ElasticsearchRef = commonv1.ObjectSelector{SecretName: "esname"}
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "invalid-secret-es-ref-with-name",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				ent.Spec.ElasticsearchRef = commonv1.ObjectSelector{SecretName: "esname", Name: "esname"}
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookFailed(
				`spec.elasticsearchRef: Forbidden: Invalid association reference: specify name or secretName, not both`,
			),
		},
		{
			Name:      "invalid-secret-es-ref-with-namespace",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				ent.Spec.ElasticsearchRef = commonv1.ObjectSelector{SecretName: "esname", Namespace: "esns"}
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookFailed(
				`spec.elasticsearchRef: Forbidden: Invalid association reference: serviceName or namespace can only be used in combination with name, not with secretName`,
			),
		},
		{
			Name:      "invalid-secret-es-ref-with-service",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkEnterpriseSearch(uid)
				ent.Spec.ElasticsearchRef = commonv1.ObjectSelector{SecretName: "esname", ServiceName: "essvc"}
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookFailed(
				`spec.elasticsearchRef: Forbidden: Invalid association reference: serviceName or namespace can only be used in combination with name, not with secretName`,
			),
		},
	}

	validator := &entv1.EnterpriseSearch{}
	gvk := metav1.GroupVersionKind{Group: entv1.GroupVersion.Group, Version: entv1.GroupVersion.Version, Kind: entv1.Kind}
	test.RunValidationWebhookTests(t, gvk, validator, testCases...)
}

func mkEnterpriseSearch(uid string) *entv1.EnterpriseSearch {
	return &entv1.EnterpriseSearch{
		ObjectMeta: metav1.ObjectMeta{
			Name: "webhook-test",
			UID:  types.UID(uid),
		},
		Spec: entv1.EnterpriseSearchSpec{
			Version: "7.7.0",
		},
	}
}

func serialize(t *testing.T, ent *entv1.EnterpriseSearch) []byte {
	t.Helper()

	objBytes, err := json.Marshal(ent)
	require.NoError(t, err)

	return objBytes
}
