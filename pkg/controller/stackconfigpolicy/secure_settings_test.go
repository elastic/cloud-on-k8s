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
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
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

	kibana := &kbv1.Kibana{Name: "test-kb", Namespace: "test-kb-ns"}

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
		Name: "same-ns-scp", Namespace: "test-kb-ns",
		Spec: policyv1alpha1.StackConfigPolicySpec{
			ResourceSelector: metav1.LabelSelector{},
			Kibana: policyv1alpha1.KibanaConfigPolicySpec{
				SecureSettings: []commonv1.SecretSource{{SecretName: "shared-secret"}},
			},
		},
	}

	// Operator-namespace SCP with an empty selector (matches any resource), declaring "shared-secret".
	operatorSCP := &policyv1alpha1.StackConfigPolicy{
		Name: "op-scp", Namespace: "operator-ns",
		Spec: policyv1alpha1.StackConfigPolicySpec{
			ResourceSelector: metav1.LabelSelector{},
			Kibana: policyv1alpha1.KibanaConfigPolicySpec{
				SecureSettings: []commonv1.SecretSource{{SecretName: "shared-secret"}},
			},
		},
	}

	// Operator-namespace SCP that projects a single key of "central-secret" into the Kibana keystore.
	operatorSCPWithEntries := &policyv1alpha1.StackConfigPolicy{
		Name: "op-scp-entries", Namespace: "operator-ns",
		Spec: policyv1alpha1.StackConfigPolicySpec{
			ResourceSelector: metav1.LabelSelector{},
			Kibana: policyv1alpha1.KibanaConfigPolicySpec{
				SecureSettings: []commonv1.SecretSource{{
					SecretName: "central-secret",
					Entries:    []commonv1.KeyToPath{{Key: "k1"}, {Key: "k2", Path: "p2"}},
				}},
			},
		},
	}

	elasticsearchSecretWithoutEntries := &corev1.Secret{
		Name: "test-es-es-file-settings", Namespace: "test-es-ns",
	}
	setSecureSettingsAnnotation(elasticsearchSecretWithoutEntries, `[{"namespace":"operator-ns","secretName":"central-secret"}]`)

	elasticsearchSecretWithEntries := &corev1.Secret{
		Name: "test-es-es-file-settings", Namespace: "test-es-ns",
	}
	setSecureSettingsAnnotation(elasticsearchSecretWithEntries, `[{"namespace":"operator-ns","secretName":"central-secret","entries":[{"key":"k1"}]}]`)

	// Operator-namespace SCP that projects a single key of "central-secret" through each Elasticsearch secure settings field.
	operatorESSCPWithEntries := &policyv1alpha1.StackConfigPolicy{
		Name: "op-es-scp-entries", Namespace: "operator-ns",
		Spec: policyv1alpha1.StackConfigPolicySpec{
			ResourceSelector: metav1.LabelSelector{},
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SecureSettings: []commonv1.SecretSource{{SecretName: "central-secret", Entries: []commonv1.KeyToPath{{Key: "k1"}}}},
			},
		},
	}
	operatorDeprecatedSCPWithEntries := &policyv1alpha1.StackConfigPolicy{
		Name: "op-deprecated-scp-entries", Namespace: "operator-ns",
		Spec: policyv1alpha1.StackConfigPolicySpec{
			ResourceSelector: metav1.LabelSelector{},
			SecureSettings:   []commonv1.SecretSource{{SecretName: "central-secret", Entries: []commonv1.KeyToPath{{Key: "k1"}}}},
		},
	}

	elasticsearchSecret := &corev1.Secret{
		Name: "test-es-es-file-settings", Namespace: "test-es-ns",
	}
	addSecureSettingsAnnotationToSecret(elasticsearchSecret, "test-es-ns", "shared-secret")

	esSCP := &policyv1alpha1.StackConfigPolicy{
		Name: "es-scp", Namespace: "test-es-ns",
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
				resourceKind:      kbv1.Kind,
				client:            k8s.NewFakeClient(kibanaConfigSameNs, sameNsSCP),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{{SecretName: "shared-secret", Namespace: "test-kb-ns"}},
		},
		{
			name: "Kibana: source rejected when no governing SCP declares it",
			args: args{
				resource:          kibana,
				resourceKind:      kbv1.Kind,
				client:            k8s.NewFakeClient(kibanaConfigSameNs),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			name: "Kibana: source in operator namespace allowed when operator SCP declares it",
			args: args{
				resource:          kibana,
				resourceKind:      kbv1.Kind,
				client:            k8s.NewFakeClient(kibanaConfigOpNs, operatorSCP),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{{SecretName: "shared-secret", Namespace: "operator-ns"}},
		},
		{
			name: "Kibana: source in operator namespace rejected when no operator SCP is active",
			args: args{
				resource:          kibana,
				resourceKind:      kbv1.Kind,
				client:            k8s.NewFakeClient(kibanaConfigOpNs),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			name: "Kibana: source with correct namespace but undeclared secret name is rejected",
			args: args{
				resource:          kibana,
				resourceKind:      kbv1.Kind,
				client:            k8s.NewFakeClient(kibanaConfigOpNsWrongName, operatorSCP),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			name: "Kibana: cross-namespace source is always rejected",
			args: args{
				resource:          kibana,
				resourceKind:      kbv1.Kind,
				client:            k8s.NewFakeClient(kibanaConfigCrossNs),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			name: "Elasticsearch: source declared by governing SCP is allowed",
			args: args{
				resource:          &esv1.Elasticsearch{Name: "test-es", Namespace: "test-es-ns"},
				resourceKind:      esv1.Kind,
				client:            k8s.NewFakeClient(elasticsearchSecret, esSCP),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{{SecretName: "shared-secret", Namespace: "test-es-ns"}},
		},
		{
			name: "Elasticsearch: source rejected when no governing SCP declares it",
			args: args{
				resource:          &esv1.Elasticsearch{Name: "test-es", Namespace: "test-es-ns"},
				resourceKind:      esv1.Kind,
				client:            k8s.NewFakeClient(elasticsearchSecret),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			name: "Kibana: source with entries identical to the governing SCP is allowed",
			args: args{
				resource:     kibana,
				resourceKind: kbv1.Kind,
				client: k8s.NewFakeClient(
					mkKibanaConfigSecretWithAnnotation(`[{"namespace":"operator-ns","secretName":"central-secret","entries":[{"key":"k1"},{"key":"k2","path":"p2"}]}]`),
					operatorSCPWithEntries,
				),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{{
				Namespace: "operator-ns", SecretName: "central-secret",
				Entries: []commonv1.KeyToPath{{Key: "k1"}, {Key: "k2", Path: "p2"}},
			}},
		},
		{
			name: "Kibana: source with entries removed is rejected",
			args: args{
				resource:     kibana,
				resourceKind: kbv1.Kind,
				client: k8s.NewFakeClient(
					mkKibanaConfigSecretWithAnnotation(`[{"namespace":"operator-ns","secretName":"central-secret"}]`),
					operatorSCPWithEntries,
				),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			name: "Kibana: source with an added entry is rejected",
			args: args{
				resource:     kibana,
				resourceKind: kbv1.Kind,
				client: k8s.NewFakeClient(
					mkKibanaConfigSecretWithAnnotation(`[{"namespace":"operator-ns","secretName":"central-secret","entries":[{"key":"k1"},{"key":"k2","path":"p2"},{"key":"k3"}]}]`),
					operatorSCPWithEntries,
				),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			name: "Kibana: source with a replaced entry key is rejected",
			args: args{
				resource:     kibana,
				resourceKind: kbv1.Kind,
				client: k8s.NewFakeClient(
					mkKibanaConfigSecretWithAnnotation(`[{"namespace":"operator-ns","secretName":"central-secret","entries":[{"key":"k1"},{"key":"k3","path":"p2"}]}]`),
					operatorSCPWithEntries,
				),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			name: "Kibana: source with a modified entry path is rejected",
			args: args{
				resource:     kibana,
				resourceKind: kbv1.Kind,
				client: k8s.NewFakeClient(
					mkKibanaConfigSecretWithAnnotation(`[{"namespace":"operator-ns","secretName":"central-secret","entries":[{"key":"k1"},{"key":"k2","path":"other"}]}]`),
					operatorSCPWithEntries,
				),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			name: "Kibana: source with reordered entries is rejected",
			args: args{
				resource:     kibana,
				resourceKind: kbv1.Kind,
				client: k8s.NewFakeClient(
					mkKibanaConfigSecretWithAnnotation(`[{"namespace":"operator-ns","secretName":"central-secret","entries":[{"key":"k2","path":"p2"},{"key":"k1"}]}]`),
					operatorSCPWithEntries,
				),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			name: "Elasticsearch: source with entries identical to the governing SCP is allowed",
			args: args{
				resource:          &esv1.Elasticsearch{Name: "test-es", Namespace: "test-es-ns"},
				resourceKind:      esv1.Kind,
				client:            k8s.NewFakeClient(elasticsearchSecretWithEntries, operatorESSCPWithEntries),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{{Namespace: "operator-ns", SecretName: "central-secret", Entries: []commonv1.KeyToPath{{Key: "k1"}}}},
		},
		{
			name: "Elasticsearch: source with entries removed is rejected",
			args: args{
				resource:          &esv1.Elasticsearch{Name: "test-es", Namespace: "test-es-ns"},
				resourceKind:      esv1.Kind,
				client:            k8s.NewFakeClient(elasticsearchSecretWithoutEntries, operatorESSCPWithEntries),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			name: "Elasticsearch: source with entries identical to the deprecated SCP secure settings is allowed",
			args: args{
				resource:          &esv1.Elasticsearch{Name: "test-es", Namespace: "test-es-ns"},
				resourceKind:      esv1.Kind,
				client:            k8s.NewFakeClient(elasticsearchSecretWithEntries, operatorDeprecatedSCPWithEntries),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{{Namespace: "operator-ns", SecretName: "central-secret", Entries: []commonv1.KeyToPath{{Key: "k1"}}}},
		},
		{
			name: "Elasticsearch: source with entries removed from the deprecated SCP secure settings is rejected",
			args: args{
				resource:          &esv1.Elasticsearch{Name: "test-es", Namespace: "test-es-ns"},
				resourceKind:      esv1.Kind,
				client:            k8s.NewFakeClient(elasticsearchSecretWithoutEntries, operatorDeprecatedSCPWithEntries),
				operatorNamespace: "operator-ns",
			},
			want: []commonv1.NamespacedSecretSource{},
		},
		{
			// Two SCPs declare the same secret with different Entries. The annotation contains
			// both sources, and each one matches the entries declared by one of the SCPs.
			name: "Kibana: two SCPs with diverging Entries — each annotation source matching an SCP passes",
			args: args{
				resource:     kibana,
				resourceKind: kbv1.Kind,
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
						Name: "scp-a", Namespace: "test-kb-ns",
						Spec: policyv1alpha1.StackConfigPolicySpec{
							ResourceSelector: metav1.LabelSelector{},
							Kibana: policyv1alpha1.KibanaConfigPolicySpec{
								SecureSettings: []commonv1.SecretSource{{SecretName: "shared-secret", Entries: []commonv1.KeyToPath{{Key: "k1"}}}},
							},
						},
					},
					&policyv1alpha1.StackConfigPolicy{
						Name: "scp-b", Namespace: "test-kb-ns",
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

func Test_allowedSources(t *testing.T) {
	policy := &policyv1alpha1.StackConfigPolicy{
		Name: "scp", Namespace: "ns",
		Spec: policyv1alpha1.StackConfigPolicySpec{
			SecureSettings: []commonv1.SecretSource{{SecretName: "deprecated-secret", Entries: []commonv1.KeyToPath{{Key: "d1"}}}},
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SecureSettings: []commonv1.SecretSource{{SecretName: "es-secret", Entries: []commonv1.KeyToPath{{Key: "e1", Path: "p1"}}}},
			},
			Kibana: policyv1alpha1.KibanaConfigPolicySpec{
				SecureSettings: []commonv1.SecretSource{{SecretName: "kb-secret", Entries: []commonv1.KeyToPath{{Key: "k1"}}}},
			},
		},
	}

	tests := []struct {
		name         string
		resourceKind string
		want         []commonv1.NamespacedSecretSource
	}{
		{
			name:         "Elasticsearch includes both Spec.Elasticsearch.SecureSettings and deprecated Spec.SecureSettings with their entries",
			resourceKind: esv1.Kind,
			want: []commonv1.NamespacedSecretSource{
				{Namespace: "ns", SecretName: "deprecated-secret", Entries: []commonv1.KeyToPath{{Key: "d1"}}},
				{Namespace: "ns", SecretName: "es-secret", Entries: []commonv1.KeyToPath{{Key: "e1", Path: "p1"}}},
			},
		},
		{
			name:         "Kibana includes only Spec.Kibana.SecureSettings with their entries",
			resourceKind: kbv1.Kind,
			want:         []commonv1.NamespacedSecretSource{{Namespace: "ns", SecretName: "kb-secret", Entries: []commonv1.KeyToPath{{Key: "k1"}}}},
		},
		{
			name:         "unknown kind returns no sources",
			resourceKind: "Beat",
			want:         nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, allowedSources(policy, tt.resourceKind))
		})
	}
}

func Test_isSourceAllowed(t *testing.T) {
	tests := []struct {
		name    string
		allowed []commonv1.NamespacedSecretSource
		src     commonv1.NamespacedSecretSource
		want    bool
	}{
		{
			name:    "empty entries in the policy match the annotation that omits them",
			allowed: []commonv1.NamespacedSecretSource{{Namespace: "ns", SecretName: "s", Entries: []commonv1.KeyToPath{}}},
			src:     commonv1.NamespacedSecretSource{Namespace: "ns", SecretName: "s"},
			want:    true,
		},
		{
			name:    "same entries match",
			allowed: []commonv1.NamespacedSecretSource{{Namespace: "ns", SecretName: "s", Entries: []commonv1.KeyToPath{{Key: "k1", Path: "p1"}}}},
			src:     commonv1.NamespacedSecretSource{Namespace: "ns", SecretName: "s", Entries: []commonv1.KeyToPath{{Key: "k1", Path: "p1"}}},
			want:    true,
		},
		{
			name:    "omitted entries do not match declared entries",
			allowed: []commonv1.NamespacedSecretSource{{Namespace: "ns", SecretName: "s", Entries: []commonv1.KeyToPath{{Key: "k1"}}}},
			src:     commonv1.NamespacedSecretSource{Namespace: "ns", SecretName: "s"},
			want:    false,
		},
		{
			name:    "different namespace does not match",
			allowed: []commonv1.NamespacedSecretSource{{Namespace: "ns", SecretName: "s"}},
			src:     commonv1.NamespacedSecretSource{Namespace: "other", SecretName: "s"},
			want:    false,
		},
		{
			name:    "different secret name does not match",
			allowed: []commonv1.NamespacedSecretSource{{Namespace: "ns", SecretName: "s"}},
			src:     commonv1.NamespacedSecretSource{Namespace: "ns", SecretName: "other"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isSourceAllowed(tt.allowed, tt.src))
		})
	}
}

func addSecureSettingsAnnotationToSecret(secret *corev1.Secret, namespace, secretName string) {
	setSecureSettingsAnnotation(secret, fmt.Sprintf(`[{"namespace":"%s","secretName":"%s"}]`, namespace, secretName))
}

func setSecureSettingsAnnotation(secret *corev1.Secret, sources string) {
	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}
	secret.Annotations["policy.k8s.elastic.co/secure-settings-secrets"] = sources
}

func mkKibanaConfigSecretWithAnnotation(sources string) *corev1.Secret {
	secret := MkKibanaConfigSecret("test-kb-ns", "test-policy", "test-policy-ns", "")
	setSecureSettingsAnnotation(secret, sources)
	return secret
}
