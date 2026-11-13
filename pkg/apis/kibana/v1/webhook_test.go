// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/test"
)

func TestWebhook(t *testing.T) {
	testCases := []test.ValidationWebhookTestCase{
		{
			Name:      "create-valid",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				return test.MustMarshalJSON(t, k)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "unknown-field",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.SetAnnotations(map[string]string{
					corev1.LastAppliedConfigAnnotation: `{"metadata":{"name": "ekesn", "namespace": "default", "uid": "e7a18cfb-b017-475c-8da2-1ec941b1f285", "creationTimestamp":"2020-03-24T13:43:20Z" },"spec":{"version":"7.6.1", "unknown": "UNKNOWN"}}`,
				})
				return test.MustMarshalJSON(t, k)
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
				k := mkKibana(uid)
				k.SetName(strings.Repeat("x", 100))
				return test.MustMarshalJSON(t, k)
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
				k := mkKibana(uid)
				k.Spec.Version = "7.x"
				return test.MustMarshalJSON(t, k)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "7.x": Invalid version: No Major.Minor.Patch elements found`,
			),
		},
		{
			Name:      "deprecated-version",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "7.10.0"
				return test.MustMarshalJSON(t, k)
			},
			Check: test.ValidationWebhookSucceededWithWarnings(
				`Version 7.10.0 is EOL and support for it will be removed in a future release of the ECK operator`,
			),
		},
		{
			Name:      "deprecated-at-lowest-supported-7-1-0",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "7.1.0"
				return test.MustMarshalJSON(t, k)
			},
			Check: test.ValidationWebhookSucceededWithWarnings(
				`Version 7.1.0 is EOL and support for it will be removed in a future release of the ECK operator`,
			),
		},
		{
			Name:      "create-8-0-0-no-deprecation-warning",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.0.0"
				return test.MustMarshalJSON(t, k)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "update-deprecated-same-version-label-change-still-warns",
			Operation: admissionv1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "7.10.0"
				return test.MustMarshalJSON(t, k)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "7.10.0"
				k.Labels = map[string]string{"warmed": "restart"}
				return test.MustMarshalJSON(t, k)
			},
			Check: test.ValidationWebhookSucceededWithWarnings(
				`Version 7.10.0 is EOL and support for it will be removed in a future release of the ECK operator`,
			),
		},
		{
			Name:      "update-from-deprecated-to-supported-clears-deprecation-warning",
			Operation: admissionv1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "7.10.0"
				return test.MustMarshalJSON(t, k)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				return test.MustMarshalJSON(t, k)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "unsupported-version-lower",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "3.1.2"
				return test.MustMarshalJSON(t, k)
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
				k := mkKibana(uid)
				k.Spec.Version = "300.1.2"
				return test.MustMarshalJSON(t, k)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "300.1.2": Unsupported version: version 300.1.2 is higher than the highest supported version`,
			),
		},
		{
			Name:      "update-valid",
			Operation: admissionv1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.5.1"
				return test.MustMarshalJSON(t, k)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.6.1"
				return test.MustMarshalJSON(t, k)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "version-downgrade",
			Operation: admissionv1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.6.1"
				return test.MustMarshalJSON(t, k)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.5.1"
				return test.MustMarshalJSON(t, k)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Forbidden: Version downgrades are not supported`,
			),
		},
		{
			Name:      "version-downgrade-with-override",
			Operation: admissionv1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.6.1"
				return test.MustMarshalJSON(t, k)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.5.1"
				k.Annotations = map[string]string{
					commonv1.DisableDowngradeValidationAnnotation: "true",
				}
				return test.MustMarshalJSON(t, k)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "named-es-ref",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkKibana(uid)
				ent.Spec.ElasticsearchRef = commonv1.ElasticsearchSelector{
					Name: "esname",
				}
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "named-es-ref-with-namesapce",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkKibana(uid)
				ent.Spec.ElasticsearchRef = commonv1.ElasticsearchSelector{
					Name: "esname", Namespace: "esns",
				}
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "named-es-ref-with-servicename",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkKibana(uid)
				ent.Spec.ElasticsearchRef = commonv1.ElasticsearchSelector{
					Name: "esname", ServiceName: "esns",
				}
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "secret-es-ref",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkKibana(uid)
				ent.Spec.ElasticsearchRef = commonv1.ElasticsearchSelector{
					SecretName: "esname",
				}
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "invalid-secret-es-ref-secret-and-name",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				kb := mkKibana(uid)
				kb.Spec.ElasticsearchRef = commonv1.ElasticsearchSelector{
					SecretName: "esname", Name: "esname",
				}
				return serialize(t, kb)
			},
			Check: test.ValidationWebhookFailed(
				`spec.elasticsearchRef: Forbidden: Invalid association reference: specify name or secretName, not both`,
			),
		},
		{
			Name:      "deprecated-version-invalid-es-ref-warning-and-denial",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				kb := mkKibana(uid)
				kb.Spec.Version = "7.10.0"
				kb.Spec.ElasticsearchRef = commonv1.ElasticsearchSelector{
					SecretName: "esname", Name: "esname",
				}
				return serialize(t, kb)
			},
			Check: test.ValidationWebhookFailedWithWarnings(
				[]string{`spec.elasticsearchRef: Forbidden: Invalid association reference: specify name or secretName, not both`},
				[]string{`Version 7.10.0 is EOL and support for it will be removed in a future release of the ECK operator`},
			),
		},
		{
			Name:      "invalid-secret-es-ref-secret-and-namespace",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				kb := mkKibana(uid)
				kb.Spec.ElasticsearchRef = commonv1.ElasticsearchSelector{
					SecretName: "esname", Namespace: "esns",
				}
				return serialize(t, kb)
			},
			Check: test.ValidationWebhookFailed(
				`spec.elasticsearchRef: Forbidden: Invalid association reference: serviceName or namespace can only be used in combination with name, not with secretName`,
			),
		},
		{
			Name:      "invalid-secret-es-ref-secret-and-service",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				kb := mkKibana(uid)
				kb.Spec.ElasticsearchRef = commonv1.ElasticsearchSelector{
					SecretName: "esname", ServiceName: "esname",
				}
				return serialize(t, kb)
			},
			Check: test.ValidationWebhookFailed(
				`spec.elasticsearchRef: Forbidden: Invalid association reference: serviceName or namespace can only be used in combination with name, not with secretName`,
			),
		},
		{
			Name:      "simple-stackmon-ref",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkKibana(uid)
				ent.Spec.Version = "7.17.0"
				ent.Spec.Monitoring = commonv1.Monitoring{Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "esmonname", Namespace: "esmonns"}}}}
				ent.Spec.ElasticsearchRef = commonv1.ElasticsearchSelector{
					Name: "esname", Namespace: "esns",
				}
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "multiple-stackmon-ref",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkKibana(uid)
				ent.Spec.Version = "7.17.0"
				ent.Spec.Monitoring = commonv1.Monitoring{
					Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es1monname"}}},
					Logs:    commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es2monname"}}},
				}
				ent.Spec.ElasticsearchRef = commonv1.ElasticsearchSelector{
					Name: "esname", Namespace: "esns",
				}
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "invalid-version-for-stackmon",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkKibana(uid)
				ent.Spec.Version = "7.13.0"
				ent.Spec.Monitoring = commonv1.Monitoring{Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "esmonname", Namespace: "esmonns"}}}}
				ent.Spec.ElasticsearchRef = commonv1.ElasticsearchSelector{
					Name: "esname", Namespace: "esns",
				}
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "7.13.0": Unsupported version for Stack Monitoring. Required >= 7.14.0.`,
			),
		},
		{
			Name:      "invalid-stackmon-ref-with-name",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkKibana(uid)
				ent.Spec.Version = "7.17.0"
				ent.Spec.Monitoring = commonv1.Monitoring{
					Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es1monname", Name: "xx"}}},
					Logs:    commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es2monname"}}},
				}
				ent.Spec.ElasticsearchRef = commonv1.ElasticsearchSelector{
					Name: "esname", Namespace: "esns",
				}
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookFailed(
				`spec.monitoring.metrics: Forbidden: Invalid association reference: specify name or secretName, not both`,
			),
		},
		{
			Name:      "invalid-stackmon-ref-with-service-name",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkKibana(uid)
				ent.Spec.Version = "7.17.0"
				ent.Spec.Monitoring = commonv1.Monitoring{
					Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es1monname"}}},
					Logs:    commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es2monname", ServiceName: "xx"}}},
				}
				ent.Spec.ElasticsearchRef = commonv1.ElasticsearchSelector{
					Name: "esname", Namespace: "esns",
				}
				return serialize(t, ent)
			},
			Check: test.ValidationWebhookFailed(
				`spec.monitoring.logs: Forbidden: Invalid association reference: serviceName or namespace can only be used in combination with name, not with secretName`,
			),
		},
		{
			Name:      "simple-epr-ref",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkKibana(uid)
				ent.Spec.PackageRegistryRef = commonv1.LocalObjectSelector{Name: "epr", Namespace: "esns"}
				return test.MustMarshalJSON(t, ent)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "pause-orchestration false",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkKibana(uid)
				ent.Annotations[commonv1.PauseOrchestrationAnnotation] = "false"
				return test.MustMarshalJSON(t, ent)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "pause-orchestration true",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkKibana(uid)
				ent.Annotations[commonv1.PauseOrchestrationAnnotation] = "true"
				return test.MustMarshalJSON(t, ent)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "pause-orchestration invalid",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ent := mkKibana(uid)
				ent.Annotations[commonv1.PauseOrchestrationAnnotation] = "True"
				return test.MustMarshalJSON(t, ent)
			},
			Check: test.ValidationWebhookFailed("must be set to either 'true' or 'false' if provided"),
		},
		// --- background tasks checks ---
		{
			Name:      "backgroundTasks-valid",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				count := int32(2)
				k.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{Count: &count}
				return test.MustMarshalJSON(t, k)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "backgroundTasks-version-too-old",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.15.0"
				k.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{}
				return test.MustMarshalJSON(t, k)
			},
			Check: test.ValidationWebhookFailed("spec.backgroundTasks requires Kibana >= 8.16.0"),
		},
		{
			Name:      "backgroundTasks-node-roles-in-spec-config-forbidden",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				k.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{}
				k.Spec.Config = &commonv1.Config{
					Data: map[string]any{"node.roles": []string{"ui"}},
				}
				return serialize(t, k)
			},
			Check: test.ValidationWebhookFailed("node.roles is managed by ECK"),
		},
		{
			Name:      "backgroundTasks-node-roles-in-bg-config-forbidden",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				k.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{
					Config: &commonv1.Config{
						Data: map[string]any{"node.roles": []string{"background_tasks"}},
					},
				}
				return serialize(t, k)
			},
			Check: test.ValidationWebhookFailed("node.roles is managed by ECK"),
		},
		{
			Name:      "backgroundTasks-node-roles-in-bg-config-forbidden-canonical",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				k.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{
					Config: &commonv1.Config{
						Data: map[string]any{"node": map[string]any{"roles": []string{"background_tasks"}}},
					},
				}
				return serialize(t, k)
			},
			Check: test.ValidationWebhookFailed("node.roles is managed by ECK"),
		},
		{
			Name:      "backgroundTasks-node-roles-env-var-in-spec-pod-template-forbidden",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				k.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{}
				k.Spec.PodTemplate = corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name: kbv1.KibanaContainerName,
						Env:  []corev1.EnvVar{{Name: "NODE_ROLES", Value: `["ui"]`}},
					}}},
				}
				return serialize(t, k)
			},
			Check: test.ValidationWebhookFailed("must not be set via the NODE_ROLES env var in spec.podTemplate"),
		},
		{
			Name:      "backgroundTasks-node-roles-env-var-in-bg-pod-template-forbidden",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				k.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{Containers: []corev1.Container{{
							Name: kbv1.KibanaContainerName,
							Env:  []corev1.EnvVar{{Name: "NODE_ROLES", Value: `["background_tasks"]`}},
						}}},
					},
				}
				return serialize(t, k)
			},
			Check: test.ValidationWebhookFailed("must not be set via the NODE_ROLES env var in spec.backgroundTasks.podTemplate"),
		},
		{
			Name:      "backgroundTasks-node-roles-env-var-in-other-container-allowed",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				k.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{}
				// NODE_ROLES on a sidecar container is not a Kibana node.roles override.
				k.Spec.PodTemplate = corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name: "sidecar",
						Env:  []corev1.EnvVar{{Name: "NODE_ROLES", Value: `["ui"]`}},
					}}},
				}
				return serialize(t, k)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "backgroundTasks-node-roles-env-var-without-split-allowed",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				k.Spec.PodTemplate = corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name: kbv1.KibanaContainerName,
						Env:  []corev1.EnvVar{{Name: "NODE_ROLES", Value: `["ui"]`}},
					}}},
				}
				return serialize(t, k)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "backgroundTasks-nil-count-allowed-for-hpa",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				// nil Count means user-managed replicas (HPA scenario).
				k.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{}
				return test.MustMarshalJSON(t, k)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "backgroundTasks-base-path-differs-in-bg-config-forbidden",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				k.Spec.Config = &commonv1.Config{
					Data: map[string]any{"server.basePath": "/primary", "server.rewriteBasePath": true},
				}
				k.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{
					Config: &commonv1.Config{
						Data: map[string]any{"server.basePath": "/tasks", "server.rewriteBasePath": true},
					},
				}
				return serialize(t, k)
			},
			Check: test.ValidationWebhookFailed("must use the same base path as the primary pool"),
		},
		{
			Name:      "backgroundTasks-base-path-only-on-bg-forbidden",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				k.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{
					Config: &commonv1.Config{
						Data: map[string]any{"server": map[string]any{"basePath": "/tasks", "rewriteBasePath": true}},
					},
				}
				return serialize(t, k)
			},
			Check: test.ValidationWebhookFailed("must use the same base path as the primary pool"),
		},
		{
			Name:      "backgroundTasks-base-path-differs-in-bg-pod-template-env-forbidden",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				k.Spec.Config = &commonv1.Config{
					Data: map[string]any{"server.basePath": "/primary", "server.rewriteBasePath": true},
				}
				k.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{
					PodTemplate: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name: kbv1.KibanaContainerName,
						Env: []corev1.EnvVar{
							{Name: "SERVER_BASEPATH", Value: "/tasks"},
							{Name: "SERVER_REWRITEBASEPATH", Value: "true"},
						},
					}}}},
				}
				return serialize(t, k)
			},
			Check: test.ValidationWebhookFailed("must use the same base path as the primary pool"),
		},
		{
			Name:      "backgroundTasks-rewrite-disabled-in-bg-config-forbidden",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				k.Spec.Config = &commonv1.Config{
					Data: map[string]any{"server.basePath": "/primary", "server.rewriteBasePath": true},
				}
				k.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{
					Config: &commonv1.Config{
						Data: map[string]any{"server.rewriteBasePath": false},
					},
				}
				return serialize(t, k)
			},
			Check: test.ValidationWebhookFailed("must use the same base path as the primary pool"),
		},
		{
			Name:      "backgroundTasks-same-base-path-repeated-in-bg-but-different-rewrite-not-allowed",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				k.Spec.Config = &commonv1.Config{
					Data: map[string]any{"server.basePath": "/primary", "server.rewriteBasePath": true},
				}
				k.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{
					Config: &commonv1.Config{
						Data: map[string]any{"server.basePath": "/primary"},
					},
				}
				return serialize(t, k)
			},
			Check: test.ValidationWebhookFailed("must use the same base path as the primary pool"),
		},
		{
			Name:      "backgroundTasks-base-path-inherited-from-primary-allowed",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				k.Spec.Config = &commonv1.Config{
					Data: map[string]any{"server.basePath": "/primary", "server.rewriteBasePath": true},
				}
				k.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{}
				return serialize(t, k)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "backgroundTasks-same-base-path-via-bg-env-allowed",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				k := mkKibana(uid)
				k.Spec.Version = "8.17.0"
				k.Spec.Config = &commonv1.Config{
					Data: map[string]any{"server.basePath": "/primary", "server.rewriteBasePath": true},
				}
				k.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{
					PodTemplate: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name: kbv1.KibanaContainerName,
						Env: []corev1.EnvVar{
							{Name: "SERVER_BASEPATH", Value: "/primary"},
							{Name: "SERVER_REWRITEBASEPATH", Value: "true"},
						},
					}}}},
				}
				return serialize(t, k)
			},
			Check: test.ValidationWebhookSucceeded,
		},
	}

	handler := test.NewValidationWebhookHandler(kbv1.Validate)
	gvk := metav1.GroupVersionKind{Group: kbv1.GroupVersion.Group, Version: kbv1.GroupVersion.Version, Kind: kbv1.Kind}
	test.RunValidationWebhookTests(t, gvk, "kibanas", handler, testCases...)
}

func mkKibana(uid string) *kbv1.Kibana {
	return &kbv1.Kibana{
		Name:        "webhook-test",
		UID:         types.UID(uid),
		Annotations: make(map[string]string),
		Spec: kbv1.KibanaSpec{
			Version: "7.17.0",
		},
	}
}

func serialize(t *testing.T, k *kbv1.Kibana) []byte {
	t.Helper()

	objBytes, err := json.Marshal(k)
	require.NoError(t, err)

	return objBytes
}
