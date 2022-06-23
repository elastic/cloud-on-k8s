// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1_test

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
	kbv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1beta1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/test"
)

func TestWebhook(t *testing.T) {
	testCases := []test.ValidationWebhookTestCase{
		{
			Name:      "create-valid",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				return serialize(t, k)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "unknown-field",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.SetAnnotations(map[string]string{
					corev1.LastAppliedConfigAnnotation: `{"metadata":{"name": "ekesn", "namespace": "default", "uid": "e7a18cfb-b017-475c-8da2-1ec941b1f285", "creationTimestamp":"2020-03-24T13:43:20Z" },"spec":{"version":"7.6.1", "unknown": "UNKNOWN"}}`,
				})
				return serialize(t, k)
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
				k := mkKibana(uid)
				k.SetName(strings.Repeat("x", 100))
				return serialize(t, k)
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
				k := mkKibana(uid)
				k.Spec.Version = "7.x"
				return serialize(t, k)
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
				k := mkKibana(uid)
				k.Spec.Version = "3.1.2"
				return serialize(t, k)
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
				k := mkKibana(uid)
				k.Spec.Version = "300.1.2"
				return serialize(t, k)
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
				k := mkKibana(uid)
				k.Spec.Version = "7.5.1"
				return serialize(t, k)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "7.6.1"
				return serialize(t, k)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "version-downgrade",
			Operation: admissionv1beta1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "7.6.1"
				return serialize(t, k)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "7.5.1"
				return serialize(t, k)
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
				k := mkKibana(uid)
				k.Spec.Version = "7.6.1"
				return serialize(t, k)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "7.5.1"
				k.Annotations = map[string]string{
					commonv1.DisableDowngradeValidationAnnotation: "true",
				}
				return serialize(t, k)
			},
			Check: test.ValidationWebhookSucceeded,
		},
	}

	validator := &kbv1beta1.Kibana{}
	gvk := metav1.GroupVersionKind{Group: kbv1beta1.GroupVersion.Group, Version: kbv1beta1.GroupVersion.Version, Kind: "Kibana"}
	test.RunValidationWebhookTests(t, gvk, validator, testCases...)
}

func mkKibana(uid string) *kbv1beta1.Kibana {
	return &kbv1beta1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name: "webhook-test",
			UID:  types.UID(uid),
		},
		Spec: kbv1beta1.KibanaSpec{
			Version: "7.6.1",
		},
	}
}

func serialize(t *testing.T, k *kbv1beta1.Kibana) []byte {
	t.Helper()

	objBytes, err := json.Marshal(k)
	require.NoError(t, err)

	return objBytes
}
