// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package substitution

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

const policyNamespace = "test-ns"

func configMap(name string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: policyNamespace},
		Data:       data,
	}
}

func secret(name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: policyNamespace},
		Data:       data,
	}
}

func Test_ResolveVarsAndSubstituteVars(t *testing.T) {
	for _, tc := range []struct {
		name           string
		k8sObjects     []client.Object
		sources        []policyv1alpha1.VariableSource
		spec           policyv1alpha1.ElasticsearchConfigPolicySpec
		wantResolveErr bool
		wantSubstErr   bool
		check          func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec)
	}{
		{
			name:    "no VariablesFrom: spec is returned unchanged",
			sources: nil,
			spec:    policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "value"}}},
			check: func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec) {
				t.Helper()
				assert.Equal(t, "value", result.ClusterSettings.Data["key"])
			},
		},
		{
			name:       "substitutes variables from ConfigMap",
			k8sObjects: []client.Object{configMap("vars", map[string]string{"BUCKET_NAME": "my-bucket", "REGION": "eu-west-1"})},
			sources:    []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "vars"}},
			spec: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SnapshotRepositories: &commonv1.Config{Data: map[string]any{
					"my-repo": map[string]any{"type": "s3", "settings": map[string]any{
						"bucket": "${BUCKET_NAME}",
						"region": "${REGION}",
					}},
				}},
			},
			check: func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec) {
				t.Helper()
				repoMap, ok := result.SnapshotRepositories.Data["my-repo"].(map[string]any)
				require.True(t, ok, "my-repo should be a map")
				repo, ok := repoMap["settings"].(map[string]any)
				require.True(t, ok, "settings should be a map")
				assert.Equal(t, "my-bucket", repo["bucket"])
				assert.Equal(t, "eu-west-1", repo["region"])
			},
		},
		{
			name:       "substitutes variables from Secret",
			k8sObjects: []client.Object{secret("creds", map[string][]byte{"ACCESS_KEY": []byte("AKIAIOSFODNN7")})},
			sources:    []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindSecret, Name: "creds"}},
			spec: policyv1alpha1.ElasticsearchConfigPolicySpec{
				ClusterSettings: &commonv1.Config{Data: map[string]any{"s3.access_key": "${ACCESS_KEY}"}},
			},
			check: func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec) {
				t.Helper()
				assert.Equal(t, "AKIAIOSFODNN7", result.ClusterSettings.Data["s3.access_key"])
			},
		},
		{
			name: "later source overrides earlier source on key conflict",
			k8sObjects: []client.Object{
				configMap("cm-vars", map[string]string{"BUCKET_NAME": "from-configmap"}),
				secret("sec-vars", map[string][]byte{"BUCKET_NAME": []byte("from-secret")}),
			},
			sources: []policyv1alpha1.VariableSource{
				{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "cm-vars"},
				{Kind: policyv1alpha1.VariableSourceKindSecret, Name: "sec-vars"},
			},
			spec: policyv1alpha1.ElasticsearchConfigPolicySpec{
				ClusterSettings: &commonv1.Config{Data: map[string]any{"bucket": "${BUCKET_NAME}"}},
			},
			check: func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec) {
				t.Helper()
				assert.Equal(t, "from-secret", result.ClusterSettings.Data["bucket"])
			},
		},
		{
			// The ConfigMap must exist so client.Get succeeds; an empty Data map would work too.
			// Strict mode (error on undefined variables) is active whenever the lookup function
			// returns ok=false, regardless of map size.
			name:         "undefined variable returns an error",
			k8sObjects:   []client.Object{configMap("vars", map[string]string{"DUMMY": "x"})},
			sources:      []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "vars"}},
			spec:         policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "${UNDEFINED_VAR}"}}},
			wantSubstErr: true,
		},
		{
			name:       "undefined variable with default uses the default value",
			k8sObjects: []client.Object{configMap("vars", map[string]string{"DUMMY": "x"})},
			sources:    []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "vars"}},
			spec:       policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "${OPTIONAL_VAR:-default-value}"}}},
			check: func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec) {
				t.Helper()
				assert.Equal(t, "default-value", result.ClusterSettings.Data["key"])
			},
		},
		{
			name:           "missing non-optional ConfigMap returns an error",
			sources:        []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "does-not-exist"}},
			spec:           policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "${VAR}"}}},
			wantResolveErr: true,
		},
		{
			name:           "missing non-optional Secret returns an error",
			sources:        []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindSecret, Name: "does-not-exist"}},
			spec:           policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "${VAR}"}}},
			wantResolveErr: true,
		},
		{
			name:    "optional missing ConfigMap is silently skipped; default value is used",
			sources: []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "does-not-exist", Optional: true}},
			spec:    policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "${VAR:-fallback}"}}},
			check: func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec) {
				t.Helper()
				assert.Equal(t, "fallback", result.ClusterSettings.Data["key"])
			},
		},
		{
			name:    "optional missing Secret is silently skipped; default value is used",
			sources: []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindSecret, Name: "does-not-exist", Optional: true}},
			spec:    policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "${VAR:-fallback}"}}},
			check: func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec) {
				t.Helper()
				assert.Equal(t, "fallback", result.ClusterSettings.Data["key"])
			},
		},
		{
			// Values are JSON-escaped at resolution time (ResolveVars) and must round-trip
			// back to their original form after substitution and JSON unmarshal.
			name: "values with JSON-special characters round-trip correctly",
			k8sObjects: []client.Object{secret("special-vars", map[string][]byte{
				"CERT":        []byte("-----BEGIN CERT-----\nABC\n-----END CERT-----"),
				"WIN_PATH":    []byte(`C:\Users\test`),
				"QUOTED_DESC": []byte(`say "hello"`),
			})},
			sources: []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindSecret, Name: "special-vars"}},
			spec: policyv1alpha1.ElasticsearchConfigPolicySpec{
				ClusterSettings: &commonv1.Config{Data: map[string]any{
					"cert": "${CERT}",
					"path": "${WIN_PATH}",
					"desc": "${QUOTED_DESC}",
				}},
			},
			check: func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec) {
				t.Helper()
				assert.Equal(t, "-----BEGIN CERT-----\nABC\n-----END CERT-----", result.ClusterSettings.Data["cert"])
				assert.Equal(t, `C:\Users\test`, result.ClusterSettings.Data["path"])
				assert.Equal(t, `say "hello"`, result.ClusterSettings.Data["desc"])
			},
		},
		{
			name: "resolves variables from a cross-namespace source",
			k8sObjects: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "remote-vars", Namespace: "other-ns"},
				Data:       map[string]string{"REMOTE_KEY": "remote-value"},
			}},
			sources: []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "remote-vars", Namespace: "other-ns"}},
			spec:    policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "${REMOTE_KEY}"}}},
			check: func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec) {
				t.Helper()
				assert.Equal(t, "remote-value", result.ClusterSettings.Data["key"])
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &policyv1alpha1.StackConfigPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test-policy", Namespace: policyNamespace},
				Spec:       policyv1alpha1.StackConfigPolicySpec{VariablesFrom: tc.sources},
			}
			vars, err := ResolveVars(t.Context(), k8s.NewFakeClient(tc.k8sObjects...), p)
			if tc.wantResolveErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			err = SubstituteVars(&tc.spec, vars)
			if tc.wantSubstErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, &tc.spec)
			}
		})
	}
}
