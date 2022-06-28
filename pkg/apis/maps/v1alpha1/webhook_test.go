// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1_test

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
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/test"
)

func TestWebhook(t *testing.T) {
	testCases := []test.ValidationWebhookTestCase{
		{
			Name:      "create-valid",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkMaps(uid)
				return serialize(t, m)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "unknown-field",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkMaps(uid)
				m.SetAnnotations(map[string]string{
					corev1.LastAppliedConfigAnnotation: `{"metadata":{"name": "ekesn", "namespace": "default", "uid": "e7a18cfb-b017-475c-8da2-1ec941b1f285", "creationTimestamp":"2020-03-24T13:43:20Z" },"spec":{"version":"7.6.1", "unknown": "UNKNOWN"}}`,
				})
				return serialize(t, m)
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
				m := mkMaps(uid)
				m.SetName(strings.Repeat("x", 100))
				return serialize(t, m)
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
				m := mkMaps(uid)
				m.Spec.Version = "7.x"
				return serialize(t, m)
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
				m := mkMaps(uid)
				m.Spec.Version = "3.1.2"
				return serialize(t, m)
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
				m := mkMaps(uid)
				m.Spec.Version = "300.1.2"
				return serialize(t, m)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "300.1.2": Unsupported version: version 300.1.2 is higher than the highest supported version`,
			),
		},
		{
			Name:      "named-es-ref",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkMaps(uid)
				m.Spec.Version = "7.12.0"
				m.Spec.ElasticsearchRef = commonv1.ObjectSelector{Name: "esname", Namespace: "esns", ServiceName: "essvc"}
				return serialize(t, m)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "secret-es-ref",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkMaps(uid)
				m.Spec.Version = "7.12.0"
				m.Spec.ElasticsearchRef = commonv1.ObjectSelector{SecretName: "esname"}
				return serialize(t, m)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "invalid-secret-es-ref-name",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkMaps(uid)
				m.Spec.Version = "7.12.0"
				m.Spec.ElasticsearchRef = commonv1.ObjectSelector{SecretName: "esname", Name: "esname"}
				return serialize(t, m)
			},
			Check: test.ValidationWebhookFailed(
				`spec.elasticsearchRef: Forbidden: Invalid association reference: specify name or secretName, not both`,
			),
		},
		{
			Name:      "invalid-secret-es-ref-namespace",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkMaps(uid)
				m.Spec.Version = "7.12.0"
				m.Spec.ElasticsearchRef = commonv1.ObjectSelector{SecretName: "esname", Namespace: "esname"}
				return serialize(t, m)
			},
			Check: test.ValidationWebhookFailed(
				`spec.elasticsearchRef: Forbidden: Invalid association reference: serviceName or namespace can only be used in combination with name, not with secretName`,
			),
		},
	}

	validator := &emsv1alpha1.ElasticMapsServer{}
	gvk := metav1.GroupVersionKind{Group: emsv1alpha1.GroupVersion.Group, Version: emsv1alpha1.GroupVersion.Version, Kind: emsv1alpha1.Kind}
	test.RunValidationWebhookTests(t, gvk, validator, testCases...)
}

func mkMaps(uid string) *emsv1alpha1.ElasticMapsServer {
	return &emsv1alpha1.ElasticMapsServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "webhook-test",
			UID:  types.UID(uid),
		},
		Spec: emsv1alpha1.MapsSpec{
			Version: "7.12.0",
		},
	}
}

func serialize(t *testing.T, k *emsv1alpha1.ElasticMapsServer) []byte {
	t.Helper()

	objBytes, err := json.Marshal(k)
	require.NoError(t, err)

	return objBytes
}
