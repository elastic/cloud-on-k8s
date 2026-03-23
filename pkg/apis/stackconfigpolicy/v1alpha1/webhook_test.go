// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1_test

import (
	"strings"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/test"
)

func TestWebhook(t *testing.T) {
	testCases := []test.ValidationWebhookTestCase{
		{
			Name:      "create-valid",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkStackConfigPolicy(uid)
				m.Spec.Elasticsearch.SecretMounts = []policyv1alpha1.SecretMount{
					{
						SecretName: "test1",
						MountPath:  "/usr/test1",
					},
					{
						SecretName: "test2",
						MountPath:  "/usr/test2",
					},
				}
				return test.MustMarshalJSON(t, m)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "create-valid-kibana",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkStackConfigPolicy(uid)
				m.Spec.Elasticsearch = policyv1alpha1.ElasticsearchConfigPolicySpec{}
				m.Spec.Kibana = policyv1alpha1.KibanaConfigPolicySpec{
					Config: &commonv1.Config{Data: map[string]any{"a": "b"}},
				}
				return test.MustMarshalJSON(t, m)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "unknown-field",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkStackConfigPolicy(uid)
				m.SetAnnotations(map[string]string{
					corev1.LastAppliedConfigAnnotation: `{
						"metadata":{"name": "scp", "namespace": "default", "uid": "e7a18cfb-b017-475c-8da2-1ec941b1f285", "creationTimestamp":"2020-03-24T13:43:20Z" },
						"spec":{"unknown": "blurb"}
					}`,
				})
				return test.MustMarshalJSON(t, m)
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
				m := mkStackConfigPolicy(uid)
				m.SetName(strings.Repeat("x", 100))
				return test.MustMarshalJSON(t, m)
			},
			Check: test.ValidationWebhookFailed(
				`metadata.name: Too long: may not be more than 36 bytes`,
			),
		},
		{
			Name:      "no-settings",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkStackConfigPolicy(uid)
				m.Spec.Elasticsearch = policyv1alpha1.ElasticsearchConfigPolicySpec{
					SnapshotRepositories:      nil,
					SnapshotLifecyclePolicies: &commonv1.Config{Data: nil},
					ClusterSettings:           &commonv1.Config{Data: map[string]any{}},
				}
				return test.MustMarshalJSON(t, m)
			},
			Check: test.ValidationWebhookFailed(
				"One out of Elasticsearch or Kibana settings is mandatory, both must not be empty",
			),
		},
		{
			Name:      "create-with-deprecated-secure-settings",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkStackConfigPolicy(uid)
				m.Spec.SecureSettings = []commonv1.SecretSource{
					{SecretName: "my-secret"},
				}
				return test.MustMarshalJSON(t, m)
			},
			Check: test.ValidationWebhookSucceededWithWarnings(
				policyv1alpha1.SpecSecureSettingsDeprecated,
			),
		},
		{
			Name:      "create-duplicate-mountpaths",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				m := mkStackConfigPolicy(uid)
				m.Spec.Elasticsearch.SecretMounts = []policyv1alpha1.SecretMount{
					{
						SecretName: "test1",
						MountPath:  "/usr/test",
					},
					{
						SecretName: "test2",
						MountPath:  "/usr/test",
					},
				}
				return test.MustMarshalJSON(t, m)
			},
			Check: test.ValidationWebhookFailed(
				"SecretMounts cannot have duplicate mount paths",
			),
		},
	}

	handler := test.NewValidationWebhookHandler(policyv1alpha1.Validate)
	gvk := metav1.GroupVersionKind{Group: policyv1alpha1.GroupVersion.Group, Version: policyv1alpha1.GroupVersion.Version, Kind: policyv1alpha1.Kind}
	test.RunValidationWebhookTests(t, gvk, "stackconfigpolicies", handler, testCases...)
}

func mkStackConfigPolicy(uid string) *policyv1alpha1.StackConfigPolicy {
	return &policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "config-policy-test",
			UID:  types.UID(uid),
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				ClusterSettings: &commonv1.Config{Data: map[string]any{"a": "b"}},
			},
		},
	}
}
