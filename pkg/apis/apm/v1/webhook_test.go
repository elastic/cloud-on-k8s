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

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/test"
)

func TestWebhook(t *testing.T) {
	testCases := []test.ValidationWebhookTestCase{
		{
			Name:      "create-valid",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				apm := mkApmServer(uid)
				return serialize(t, apm)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "unknown-field",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				apm := mkApmServer(uid)
				apm.SetAnnotations(map[string]string{
					corev1.LastAppliedConfigAnnotation: `{"metadata":{"name": "ekesn", "namespace": "default", "uid": "e7a18cfb-b017-475c-8da2-1ec941b1f285", "creationTimestamp":"2020-03-24T13:43:20Z" },"spec":{"version":"7.6.1", "unknown": "UNKNOWN"}}`,
				})
				return serialize(t, apm)
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
				apm := mkApmServer(uid)
				apm.SetName(strings.Repeat("x", 100))
				return serialize(t, apm)
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
				apm := mkApmServer(uid)
				apm.Spec.Version = "7.x"
				return serialize(t, apm)
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
				apm := mkApmServer(uid)
				apm.Spec.Version = "3.1.2"
				return serialize(t, apm)
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
				apm := mkApmServer(uid)
				apm.Spec.Version = "300.1.2"
				return serialize(t, apm)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "300.1.2": Unsupported version: version 300.1.2 is higher than the highest supported version`,
			),
		},
		{
			Name:      "unsupported-version-for-kibana-ref",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				apm := mkApmServer(uid)
				apm.Spec.Version = "7.4.0"
				apm.Spec.KibanaRef = commonv1.ObjectSelector{Name: "kbname", Namespace: "kbns"}
				return serialize(t, apm)
			},
			Check: test.ValidationWebhookFailed(
				`spec.kibanaRef: Forbidden: minimum required version for Kibana association is 7.5.1 but desired version is 7.4.0`,
			),
		},
		{
			Name:      "support-74-if-kibana-ref-not-set",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				apm := mkApmServer(uid)
				apm.Spec.Version = "7.4.0"
				return serialize(t, apm)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "update-valid",
			Operation: admissionv1beta1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				apm := mkApmServer(uid)
				apm.Spec.Version = "7.5.1"
				return serialize(t, apm)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				apm := mkApmServer(uid)
				apm.Spec.Version = "7.6.1"
				return serialize(t, apm)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "version-downgrade",
			Operation: admissionv1beta1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				apm := mkApmServer(uid)
				apm.Spec.Version = "7.6.1"
				return serialize(t, apm)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				apm := mkApmServer(uid)
				apm.Spec.Version = "7.5.1"
				return serialize(t, apm)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Forbidden: Version downgrades are not supported`,
			),
		},
		{
			Name:      "version-downgrade with override",
			Operation: admissionv1beta1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				apm := mkApmServer(uid)
				apm.Spec.Version = "7.6.1"
				return serialize(t, apm)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				apm := mkApmServer(uid)
				apm.Spec.Version = "7.5.1"
				apm.Annotations = map[string]string{
					commonv1.DisableDowngradeValidationAnnotation: "true",
				}
				return serialize(t, apm)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "named-es-kibana-ref",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				apm := mkApmServer(uid)
				apm.Spec.ElasticsearchRef = commonv1.ObjectSelector{Name: "esname", Namespace: "esns", ServiceName: "essvc"}
				apm.Spec.KibanaRef = commonv1.ObjectSelector{Name: "kbname", Namespace: "kbns", ServiceName: "essvc"}
				return serialize(t, apm)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "secret-named-kibana-ref",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				apm := mkApmServer(uid)
				apm.Spec.ElasticsearchRef = commonv1.ObjectSelector{Name: "esname", Namespace: "esns", ServiceName: "essvc"}
				apm.Spec.KibanaRef = commonv1.ObjectSelector{SecretName: "kbname"}
				return serialize(t, apm)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "invalid-secret-kibana-ref-with-name",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				apm := mkApmServer(uid)
				apm.Spec.KibanaRef = commonv1.ObjectSelector{SecretName: "kbname", Name: "kbname"}
				return serialize(t, apm)
			},
			Check: test.ValidationWebhookFailed(
				`spec.kibanaRef: Forbidden: Invalid association reference: specify name or secretName, not both`,
			),
		},
		{
			Name:      "invalid-secret-es-ref-with-namespace",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				apm := mkApmServer(uid)
				apm.Spec.ElasticsearchRef = commonv1.ObjectSelector{SecretName: "esname", Namespace: "esns"}
				return serialize(t, apm)
			},
			Check: test.ValidationWebhookFailed(
				`spec.elasticsearchRef: Forbidden: Invalid association reference: serviceName or namespace can only be used in combination with name, not with secretName`,
			),
		},
	}

	validator := &apmv1.ApmServer{}
	gvk := metav1.GroupVersionKind{Group: apmv1.GroupVersion.Group, Version: apmv1.GroupVersion.Version, Kind: apmv1.Kind}
	test.RunValidationWebhookTests(t, gvk, validator, testCases...)
}

func mkApmServer(uid string) *apmv1.ApmServer {
	return &apmv1.ApmServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "webhook-test",
			UID:  types.UID(uid),
		},
		Spec: apmv1.ApmServerSpec{
			Version: "7.6.1",
		},
	}
}

func serialize(t *testing.T, apm *apmv1.ApmServer) []byte {
	t.Helper()

	objBytes, err := json.Marshal(apm)
	require.NoError(t, err)

	return objBytes
}
