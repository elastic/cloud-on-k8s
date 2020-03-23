// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1beta1_test

import (
	"encoding/json"
	"strings"
	"testing"

	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/test"
	"github.com/stretchr/testify/require"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestWebhook(t *testing.T) {
	testCases := []test.ValidationWebhookTestCase{
		{
			Name:      "create-valid",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				ents := mkEnterpriseSearch(uid)
				return serialize(t, ents)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "unknown-field",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				ents := mkEnterpriseSearch(uid)
				ents.SetAnnotations(map[string]string{
					corev1.LastAppliedConfigAnnotation: `{"metadata":{"name": "ekesn", "namespace": "default", "uid": "e7a18cfb-b017-475c-8da2-1ec941b1f285", "creationTimestamp":"2020-03-24T13:43:20Z" },"spec":{"version":"7.6.1", "unknown": "UNKNOWN"}}`,
				})
				return serialize(t, ents)
			},
			Check: test.ValidationWebhookFailed(
				`"unknown": unknown field found in the kubectl.kubernetes.io/last-applied-configuration annotation is unknown`,
			),
		},
		{
			Name:      "long-name",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				ents := mkEnterpriseSearch(uid)
				ents.SetName(strings.Repeat("x", 100))
				return serialize(t, ents)
			},
			Check: test.ValidationWebhookFailed(
				`metadata.name: Too long: must have at most 36 bytes`,
			),
		},
		{
			Name:      "invalid-version",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				ents := mkEnterpriseSearch(uid)
				ents.Spec.Version = "7.x"
				return serialize(t, ents)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "7.x": Invalid version: version string has too few segments: 7.x`,
			),
		},
		{
			Name:      "unsupported-version-lower",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				ents := mkEnterpriseSearch(uid)
				ents.Spec.Version = "3.1.2"
				return serialize(t, ents)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "3.1.2": Unsupported version: version 3.1.2 is lower than the lowest supported version`,
			),
		},
		{
			Name:      "unsupported-version-higher",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				ents := mkEnterpriseSearch(uid)
				ents.Spec.Version = "300.1.2"
				return serialize(t, ents)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "300.1.2": Unsupported version: version 300.1.2 is higher than the highest supported version`,
			),
		},
		{
			Name:      "update-valid",
			Operation: admissionv1beta1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				ents := mkEnterpriseSearch(uid)
				ents.Spec.Version = "7.7.0"
				return serialize(t, ents)
			},
			Object: func(t *testing.T, uid string) []byte {
				ents := mkEnterpriseSearch(uid)
				ents.Spec.Version = "7.7.1"
				return serialize(t, ents)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "version-downgrade",
			Operation: admissionv1beta1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				ents := mkEnterpriseSearch(uid)
				ents.Spec.Version = "7.7.1"
				return serialize(t, ents)
			},
			Object: func(t *testing.T, uid string) []byte {
				ents := mkEnterpriseSearch(uid)
				ents.Spec.Version = "7.7.0"
				return serialize(t, ents)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Forbidden: Version downgrades are not supported`,
			),
		},
	}

	validator := &entsv1beta1.EnterpriseSearch{}
	gvk := metav1.GroupVersionKind{Group: entsv1beta1.GroupVersion.Group, Version: entsv1beta1.GroupVersion.Version, Kind: "EnterpriseSearch"}
	test.RunValidationWebhookTests(t, gvk, validator, testCases...)
}

func mkEnterpriseSearch(uid string) *entsv1beta1.EnterpriseSearch {
	return &entsv1beta1.EnterpriseSearch{
		ObjectMeta: metav1.ObjectMeta{
			Name: "webhook-test",
			UID:  types.UID(uid),
		},
		Spec: entsv1beta1.EnterpriseSearchSpec{
			Version: "7.7.0",
		},
	}
}

func serialize(t *testing.T, ents *entsv1beta1.EnterpriseSearch) []byte {
	t.Helper()

	objBytes, err := json.Marshal(ents)
	require.NoError(t, err)

	return objBytes
}
