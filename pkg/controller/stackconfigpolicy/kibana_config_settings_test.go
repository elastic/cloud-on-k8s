// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kibanav1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_newKibanaConfigSecret(t *testing.T) {
	type args struct {
		kb     kibanav1.Kibana
		policy *policyv1alpha1.StackConfigPolicy
	}

	tests := []struct {
		name string
		args args
		want corev1.Secret
	}{
		{
			name: "construct valid kibana config secret",
			args: args{
				kb: kibanav1.Kibana{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-kb",
						Namespace: "test-ns",
					},
				},
				policy: &policyv1alpha1.StackConfigPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-policy",
						Namespace: "test-policy-ns",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						Kibana: policyv1alpha1.KibanaConfigPolicySpec{
							Config: &commonv1.Config{
								Data: map[string]interface{}{
									"xpack.canvas.enabled": true,
								},
							},
							SecureSettings: []commonv1.SecretSource{
								{
									SecretName: "shared-secret",
								},
							},
						},
					},
				},
			},
			want: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-kb-kb-policy-config",
					Labels: map[string]string{
						"asset.policy.k8s.elastic.co/on-delete": "delete",
						"kibana.k8s.elastic.co/name":            "test-kb",
						"common.k8s.elastic.co/type":            "kibana",
						"eck.k8s.elastic.co/owner-kind":         "StackConfigPolicy",
					},
					Annotations: map[string]string{
						"policy.k8s.elastic.co/kibana-config-hash":      "3077592849",
						"policy.k8s.elastic.co/secure-settings-secrets": `[{"namespace":"test-policy-ns","secretName":"shared-secret"}]`,
						"eck.k8s.elastic.co/owner-refs":                 `["test-policy-ns/test-policy"]`,
					},
				},
				Data: map[string][]byte{
					"kibana.json": []byte(`{"xpack.canvas.enabled":true}`),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newKibanaConfigSecret(tt.args.policy.Spec.Kibana, tt.args.policy.GetKibanaNamespacedSecureSettings(), tt.args.kb, []policyv1alpha1.StackConfigPolicy{*tt.args.policy})
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_kibanaConfigApplied(t *testing.T) {
	type args struct {
		kb     kibanav1.Kibana
		policy *policyv1alpha1.StackConfigPolicy
		client k8s.Client
	}

	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "config applied successfully",
			args: args{
				kb: kibanav1.Kibana{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-kb",
						Namespace: "test-ns",
					},
				},
				policy: &policyv1alpha1.StackConfigPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-policy",
						Namespace: "test-policy-ns",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						Kibana: policyv1alpha1.KibanaConfigPolicySpec{
							Config: &commonv1.Config{
								Data: map[string]interface{}{
									"xpack.canvas.enabled": true,
								},
							},
						},
					},
				},
				client: k8s.NewFakeClient(mkKibanaPod("test-ns", true, "3077592849")),
			},
			want: true,
		},
		{
			name: "config not applied yet",
			args: args{
				kb: kibanav1.Kibana{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-kb",
						Namespace: "test-ns",
					},
				},
				policy: &policyv1alpha1.StackConfigPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-policy",
						Namespace: "test-policy-ns",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						Kibana: policyv1alpha1.KibanaConfigPolicySpec{
							Config: &commonv1.Config{
								Data: map[string]interface{}{
									"xpack.canvas.enabled": true,
								},
							},
						},
					},
				},
				client: k8s.NewFakeClient(mkKibanaPod("test-ns", false, "3077592849")),
			},
			want: false,
		},
		{
			name: "no pods running for given Kibana instance",
			args: args{
				kb: kibanav1.Kibana{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-kb",
						Namespace: "test-ns",
					},
				},
				policy: &policyv1alpha1.StackConfigPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-policy",
						Namespace: "test-policy-ns",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						Kibana: policyv1alpha1.KibanaConfigPolicySpec{
							Config: &commonv1.Config{
								Data: map[string]interface{}{
									"xpack.canvas.enabled": true,
								},
							},
						},
					},
				},
				client: k8s.NewFakeClient(),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := kibanaConfigApplied(tt.args.client, tt.args.policy.Spec.Kibana, tt.args.kb)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func mkKibanaPod(namespace string, hashapplied bool, hashValue string) *corev1.Pod {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kibana-pod",
			Namespace: namespace,
			Labels: map[string]string{
				"kibana.k8s.elastic.co/name": "test-kb",
			},
			Annotations: make(map[string]string),
		},
	}

	if hashapplied {
		pod.Annotations["policy.k8s.elastic.co/kibana-config-hash"] = hashValue
	}
	return &pod
}

func MkKibanaConfigSecret(namespace string, owningPolicyName string, owningPolicyNamespace string, hashValue string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "test-kb-kb-policy-config",
			Labels: map[string]string{
				"asset.policy.k8s.elastic.co/on-delete": "delete",
				"kibana.k8s.elastic.co/name":            "test-kb",
				"common.k8s.elastic.co/type":            "kibana",
				"eck.k8s.elastic.co/owner-kind":         "StackConfigPolicy",
			},
			Annotations: map[string]string{
				"policy.k8s.elastic.co/kibana-config-hash": hashValue,
				"eck.k8s.elastic.co/owner-refs":            `["` + owningPolicyNamespace + `/` + owningPolicyName + `"]`,
			},
		},
		Data: map[string][]byte{
			"kibana.json": []byte(`{"xpack.canvas.enabled":true}`),
		},
	}
}
