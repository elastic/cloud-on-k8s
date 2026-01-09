// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/test"
)

func TestWebhook(t *testing.T) {
	testCases := []test.ValidationWebhookTestCase{
		{
			Name:      "create-valid",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				epr := mkEPR(uid)
				return serialize(t, epr)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "unknown-field",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				epr := mkEPR(uid)
				epr.SetAnnotations(map[string]string{
					corev1.LastAppliedConfigAnnotation: `{"metadata":{"name": "test-epr", "namespace": "default", "uid": "e7a18cfb-b017-475c-8da2-1ec941b1f285", "creationTimestamp":"2020-03-24T13:43:20Z" },"spec":{"version":"8.15.0", "unknown": "UNKNOWN"}}`,
				})
				return serialize(t, epr)
			},
			Check: test.ValidationWebhookFailed(
				`"unknown": unknown field found in the kubectl.kubernetes.io/last-applied-configuration annotation is unknown`,
			),
		},
		{
			Name:      "long-name",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				epr := mkEPR(uid)
				epr.SetName(strings.Repeat("x", 100))
				return serialize(t, epr)
			},
			Check: test.ValidationWebhookFailed(
				`metadata.name: Too long: may not be more than 36 bytes`,
			),
		},
		{
			Name:      "invalid-version",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				epr := mkEPR(uid)
				epr.Spec.Version = "7.x"
				return serialize(t, epr)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "7.x": Invalid version: No Major.Minor.Patch elements found`,
			),
		},
		{
			Name:      "unsupported-version-below-minimum",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				epr := mkEPR(uid)
				epr.Spec.Version = "7.14.0"
				return serialize(t, epr)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "7.14.0": Unsupported version: version 7.14.0 is lower than the lowest supported version of 7.17.8`,
			),
		},
		{
			Name:      "unsupported-version-lower",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				epr := mkEPR(uid)
				epr.Spec.Version = "3.1.2"
				return serialize(t, epr)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "3.1.2": Unsupported version: version 3.1.2 is lower than the lowest supported version`,
			),
		},
		{
			Name:      "unsupported-version-higher",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				epr := mkEPR(uid)
				epr.Spec.Version = "300.1.2"
				return serialize(t, epr)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "300.1.2": Unsupported version: version 300.1.2 is higher than the highest supported version`,
			),
		},
	}

	validator := &eprv1alpha1.PackageRegistry{}
	gvk := metav1.GroupVersionKind{Group: eprv1alpha1.GroupVersion.Group, Version: eprv1alpha1.GroupVersion.Version, Kind: eprv1alpha1.Kind}
	test.RunValidationWebhookTests(t, gvk, validator, testCases...)
}

func mkEPR(uid string) *eprv1alpha1.PackageRegistry {
	return &eprv1alpha1.PackageRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name: "webhook-test",
			UID:  types.UID(uid),
		},
		Spec: eprv1alpha1.PackageRegistrySpec{
			Version: "8.15.0",
		},
	}
}

func serialize(t *testing.T, k *eprv1alpha1.PackageRegistry) []byte {
	t.Helper()

	objBytes, err := json.Marshal(k)
	require.NoError(t, err)

	return objBytes
}
