// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	kibanav1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_GetSecureSettingsSecretSourcesForResources(t *testing.T) {
	type args struct {
		resource          metav1.Object
		resourceKind      string
		client            k8s.Client
		operatorNamespace string
	}

	kibana := &kibanav1.Kibana{ObjectMeta: metav1.ObjectMeta{Name: "test-kb", Namespace: "test-kb-ns"}}

	// Policy config secret whose annotation references a secret in the Kibana's own namespace.
	kibanaConfigSameNs := MkKibanaConfigSecret("test-kb-ns", "test-policy", "test-policy-ns", "")
	addSecureSettingsAnnotationToSecret(kibanaConfigSameNs, "test-kb-ns", "shared-secret")

	// Policy config secret whose annotation references a secret in the operator namespace.
	kibanaConfigOpNs := MkKibanaConfigSecret("test-kb-ns", "test-policy", "test-policy-ns", "")
	addSecureSettingsAnnotationToSecret(kibanaConfigOpNs, "operator-ns", "shared-secret")

	// Policy config secret whose annotation references a secret in an unrelated namespace.
	kibanaConfigCrossNs := MkKibanaConfigSecret("test-kb-ns", "test-policy", "test-policy-ns", "")
	addSecureSettingsAnnotationToSecret(kibanaConfigCrossNs, "other-tenant-ns", "shared-secret")

	// Policy config secret with a source in the operator namespace but with a different secret name
	// than what the governing SCP declares — tests exact {namespace, secretName} validation.
	kibanaConfigOpNsWrongName := MkKibanaConfigSecret("test-kb-ns", "test-policy", "test-policy-ns", "")
	addSecureSettingsAnnotationToSecret(kibanaConfigOpNsWrongName, "operator-ns", "injected-secret")

	// SCP in the same namespace as the Kibana CR, declaring "shared-secret".
	sameNsSCP := &policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "same-ns-scp", Namespace: "test-kb-ns"},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			ResourceSelector: metav1.LabelSelector{},
			Kibana: policyv1alpha1.KibanaConfigPolicySpec{
				SecureSettings: []commonv1.SecretSource{{SecretName: "shared-secret"}},
			},
		},
	}

	// Operator-namespace SCP with an empty selector (matches any resource), declaring "shared-secret".
	operatorSCP := &policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "op-scp", Namespace: "operator-ns"},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			ResourceSelector: metav1.LabelSelector{},
			Kibana: policyv1alpha1.KibanaConfigPolicySpec{
				SecureSettings: []commonv1.SecretSource{{SecretName: "shared-secret"}},
			},
		},
	}

	elasticsearchSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-es-es-file-settings", Namespace: "test-es-ns"},
	}
	addSecureSettingsAnnotationToSecret(elasticsearchSecret, "test-es-ns", "shared-secret")

	esSCP := &policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "es-scp", Namespace: "test-es-ns"},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			ResourceSelector: metav1.LabelSelector{},
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SecureSettings: []commonv1.SecretSource{{SecretName: "shared-secret"}},
			},
		},
	}

	tests := []struct {
		name string
		args args
		want []commonv1.NamespacedSecretSource
	}{
		{
			name: "Kibana: source declared by governing SCP in resource namespace is allowed",
			args: args{
				resource:          kibana,
				resourceKind:      "Kibana",
				client:            k8s.NewFakeClient(kibanaConfigSameNs, sameNsSCP),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{{SecretName: "shared-secret", Namespace: "test-kb-ns"}},
		},
		{
			name: "Kibana: source rejected when no governing SCP declares it",
			args: args{
				resource:          kibana,
				resourceKind:      "Kibana",
				client:            k8s.NewFakeClient(kibanaConfigSameNs),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			name: "Kibana: source in operator namespace allowed when operator SCP declares it",
			args: args{
				resource:          kibana,
				resourceKind:      "Kibana",
				client:            k8s.NewFakeClient(kibanaConfigOpNs, operatorSCP),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{{SecretName: "shared-secret", Namespace: "operator-ns"}},
		},
		{
			name: "Kibana: source in operator namespace rejected when no operator SCP is active",
			args: args{
				resource:          kibana,
				resourceKind:      "Kibana",
				client:            k8s.NewFakeClient(kibanaConfigOpNs),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			name: "Kibana: source with correct namespace but undeclared secret name is rejected",
			args: args{
				resource:          kibana,
				resourceKind:      "Kibana",
				client:            k8s.NewFakeClient(kibanaConfigOpNsWrongName, operatorSCP),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			name: "Kibana: cross-namespace source is always rejected",
			args: args{
				resource:          kibana,
				resourceKind:      "Kibana",
				client:            k8s.NewFakeClient(kibanaConfigCrossNs),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			name: "Elasticsearch: source declared by governing SCP is allowed",
			args: args{
				resource:          &esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "test-es", Namespace: "test-es-ns"}},
				resourceKind:      "Elasticsearch",
				client:            k8s.NewFakeClient(elasticsearchSecret, esSCP),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{{SecretName: "shared-secret", Namespace: "test-es-ns"}},
		},
		{
			name: "Elasticsearch: source rejected when no governing SCP declares it",
			args: args{
				resource:          &esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "test-es", Namespace: "test-es-ns"}},
				resourceKind:      "Elasticsearch",
				client:            k8s.NewFakeClient(elasticsearchSecret),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			// Two SCPs declare the same secret with different Entries. The annotation contains
			// both entries. Validation only checks {namespace, secretName} — entries are the
			// SCP controller's concern and will be reconciled back. Both annotation entries pass.
			name: "Kibana: two SCPs with diverging Entries — all annotation entries with authorised secret pass",
			args: args{
				resource:     kibana,
				resourceKind: "Kibana",
				client: k8s.NewFakeClient(
					func() *corev1.Secret {
						s := MkKibanaConfigSecret("test-kb-ns", "test-policy", "test-policy-ns", "")
						if s.Annotations == nil {
							s.Annotations = make(map[string]string)
						}
						s.Annotations["policy.k8s.elastic.co/secure-settings-secrets"] = `[` +
							`{"namespace":"test-kb-ns","secretName":"shared-secret","entries":[{"key":"k1"}]},` +
							`{"namespace":"test-kb-ns","secretName":"shared-secret","entries":[{"key":"k2"}]}` +
							`]`
						return s
					}(),
					&policyv1alpha1.StackConfigPolicy{
						ObjectMeta: metav1.ObjectMeta{Name: "scp-a", Namespace: "test-kb-ns"},
						Spec: policyv1alpha1.StackConfigPolicySpec{
							ResourceSelector: metav1.LabelSelector{},
							Kibana: policyv1alpha1.KibanaConfigPolicySpec{
								SecureSettings: []commonv1.SecretSource{{SecretName: "shared-secret", Entries: []commonv1.KeyToPath{{Key: "k1"}}}},
							},
						},
					},
					&policyv1alpha1.StackConfigPolicy{
						ObjectMeta: metav1.ObjectMeta{Name: "scp-b", Namespace: "test-kb-ns"},
						Spec: policyv1alpha1.StackConfigPolicySpec{
							ResourceSelector: metav1.LabelSelector{},
							Kibana: policyv1alpha1.KibanaConfigPolicySpec{
								SecureSettings: []commonv1.SecretSource{{SecretName: "shared-secret", Entries: []commonv1.KeyToPath{{Key: "k2"}}}},
							},
						},
					},
				),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{
				{Namespace: "test-kb-ns", SecretName: "shared-secret", Entries: []commonv1.KeyToPath{{Key: "k1"}}},
				{Namespace: "test-kb-ns", SecretName: "shared-secret", Entries: []commonv1.KeyToPath{{Key: "k2"}}},
			},
		},
		{
			name: "unknown resource kind returns empty",
			args: args{
				resourceKind:      "UnknownKind",
				client:            k8s.NewFakeClient(elasticsearchSecret),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetSecureSettingsSecretSourcesForResources(context.Background(), tt.args.client, tt.args.resource, tt.args.resourceKind, tt.args.operatorNamespace)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func addSecureSettingsAnnotationToSecret(secret *corev1.Secret, namespace, secretName string) {
	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}
	secret.Annotations["policy.k8s.elastic.co/secure-settings-secrets"] = fmt.Sprintf(`[{"namespace":"%s","secretName":"%s"}]`, namespace, secretName)
}
