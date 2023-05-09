// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/test"
)

func TestWebhook(t *testing.T) {
	testCases := []test.ValidationWebhookTestCase{
		{
			Name:      "simple-stackmon-ref",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ls := mkLogstash(uid)
				ls.Spec.Version = "8.7.0"
				ls.Spec.Monitoring = commonv1.Monitoring{Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "esmonname", Namespace: "esmonns"}}}}
				return serialize(t, ls)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "multiple-stackmon-ref",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ls := mkLogstash(uid)
				ls.Spec.Version = "8.7.0"
				ls.Spec.Monitoring = commonv1.Monitoring{
					Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es1monname"}}},
					Logs:    commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es2monname"}}},
				}
				return serialize(t, ls)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "invalid-version-for-stackmon",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ls := mkLogstash(uid)
				ls.Spec.Version = "7.13.0"
				ls.Spec.Monitoring = commonv1.Monitoring{Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "esmonname", Namespace: "esmonns"}}}}
				return serialize(t, ls)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Invalid value: "7.13.0": Unsupported version for Stack Monitoring. Required >= 8.7.0`,
			),
		},
		{
			Name:      "invalid-stackmon-ref-with-name",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ls := mkLogstash(uid)
				ls.Spec.Version = "8.7.0"
				ls.Spec.Monitoring = commonv1.Monitoring{
					Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es1monname", Name: "xx"}}},
					Logs:    commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es2monname"}}},
				}
				return serialize(t, ls)
			},
			Check: test.ValidationWebhookFailed(
				`spec.monitoring.metrics: Forbidden: Invalid association reference: specify name or secretName, not both`,
			),
		},
		{
			Name:      "invalid-stackmon-ref-with-service-name",
			Operation: admissionv1beta1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				ls := mkLogstash(uid)
				ls.Spec.Version = "8.7.0"
				ls.Spec.Monitoring = commonv1.Monitoring{
					Metrics: commonv1.MetricsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es1monname"}}},
					Logs:    commonv1.LogsMonitoring{ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "es2monname", ServiceName: "xx"}}},
				}
				return serialize(t, ls)
			},
			Check: test.ValidationWebhookFailed(
				`spec.monitoring.logs: Forbidden: Invalid association reference: serviceName or namespace can only be used in combination with name, not with secretName`,
			),
		},
	}

	validator := &v1alpha1.Logstash{}
	gvk := metav1.GroupVersionKind{Group: v1alpha1.GroupVersion.Group, Version: v1alpha1.GroupVersion.Version, Kind: v1alpha1.Kind}
	test.RunValidationWebhookTests(t, gvk, validator, testCases...)
}

func mkLogstash(uid string) *v1alpha1.Logstash {
	return &v1alpha1.Logstash{
		ObjectMeta: metav1.ObjectMeta{
			Name: "webhook-test",
			UID:  types.UID(uid),
		},
		Spec: v1alpha1.LogstashSpec{
			Version: "8.6.0",
		},
	}
}

func serialize(t *testing.T, k *v1alpha1.Logstash) []byte {
	t.Helper()

	objBytes, err := json.Marshal(k)
	require.NoError(t, err)

	return objBytes
}
