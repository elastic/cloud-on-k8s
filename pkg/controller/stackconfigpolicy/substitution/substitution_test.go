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

func TestApply(t *testing.T) {
	for _, tc := range []struct {
		name              string
		k8sObjects        []client.Object
		sources           []policyv1alpha1.VariableSource
		operatorNamespace string // defaults to policyNamespace when empty
		spec              policyv1alpha1.ElasticsearchConfigPolicySpec
		wantErr           bool
		check             func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec)
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
			name:       "unknown variable passes through verbatim",
			k8sObjects: []client.Object{configMap("vars", map[string]string{"DUMMY": "x"})},
			sources:    []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "vars"}},
			spec:       policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "${UNDEFINED_VAR}"}}},
			check: func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec) {
				t.Helper()
				assert.Equal(t, "${UNDEFINED_VAR}", result.ClusterSettings.Data["key"])
			},
		},
		{
			name:       "native ES setting reference passes through verbatim",
			k8sObjects: []client.Object{configMap("vars", map[string]string{"DUMMY": "x"})},
			sources:    []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "vars"}},
			spec:       policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"path.repo": "${path.repo}/snapshots"}}},
			check: func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec) {
				t.Helper()
				assert.Equal(t, "${path.repo}/snapshots", result.ClusterSettings.Data["path.repo"])
			},
		},
		{
			name:       "ES single-colon default syntax passes through verbatim",
			k8sObjects: []client.Object{configMap("vars", map[string]string{"DUMMY": "x"})},
			sources:    []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "vars"}},
			spec:       policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "${name:default}"}}},
			check: func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec) {
				t.Helper()
				assert.Equal(t, "${name:default}", result.ClusterSettings.Data["key"])
			},
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
			// bash :- semantics: the default is used when the variable is defined but empty,
			// not only when it is unset.
			name:       "empty variable with default uses the default value",
			k8sObjects: []client.Object{configMap("vars", map[string]string{"REGION": ""})},
			sources:    []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "vars"}},
			spec:       policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "${REGION:-us-east-1}"}}},
			check: func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec) {
				t.Helper()
				assert.Equal(t, "us-east-1", result.ClusterSettings.Data["key"])
			},
		},
		{
			name:    "missing non-optional ConfigMap returns an error",
			sources: []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "does-not-exist"}},
			spec:    policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "${VAR}"}}},
			wantErr: true,
		},
		{
			name:    "missing non-optional Secret returns an error",
			sources: []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindSecret, Name: "does-not-exist"}},
			spec:    policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "${VAR}"}}},
			wantErr: true,
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
			// Values are JSON-escaped at resolution time so they round-trip correctly
			// through marshal/unmarshal even when they contain JSON-special characters
			// such as quotes, backslashes, or newlines (e.g. PEM certificates).
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
			name:       "default value containing braces is handled correctly",
			k8sObjects: []client.Object{configMap("vars", map[string]string{"DUMMY": "x"})},
			sources:    []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "vars"}},
			spec:       policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "${MISSING:-{default}}"}}},
			check: func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec) {
				t.Helper()
				assert.Equal(t, "{default}", result.ClusterSettings.Data["key"])
			},
		},
		{
			// Inline defaults are part of the policy spec and go through JSON marshal before
			// substitution, so Go's JSON encoder handles special characters automatically.
			// A default containing a double quote round-trips correctly.
			name:    "inline default with JSON-special characters round-trips correctly",
			sources: []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "does-not-exist", Optional: true}},
			spec:    policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"key": `${MISSING:-say "hello"}`}}},
			check: func(t *testing.T, result *policyv1alpha1.ElasticsearchConfigPolicySpec) {
				t.Helper()
				assert.Equal(t, `say "hello"`, result.ClusterSettings.Data["key"])
			},
		},
		{
			// A policy in the operator namespace is global and may read sources from any namespace.
			name:              "operator-namespace policy resolves variables from a cross-namespace source",
			operatorNamespace: policyNamespace, // policy IS in the operator namespace
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
		{
			// A namespace-scoped policy (not in operator namespace) must not read sources outside
			// its own namespace even when the webhook is bypassed — the read path is self-guarding.
			name:              "namespace-scoped policy is rejected when referencing a cross-namespace source",
			operatorNamespace: "operator-ns", // policy is NOT in the operator namespace
			sources:           []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "remote-vars", Namespace: "other-ns"}},
			spec:              policyv1alpha1.ElasticsearchConfigPolicySpec{ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "${REMOTE_KEY}"}}},
			wantErr:           true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			operatorNamespace := tc.operatorNamespace
			if operatorNamespace == "" {
				operatorNamespace = policyNamespace
			}
			p := &policyv1alpha1.StackConfigPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test-policy", Namespace: policyNamespace},
				Spec:       policyv1alpha1.StackConfigPolicySpec{VariablesFrom: tc.sources},
			}
			err := Apply(t.Context(), k8s.NewFakeClient(tc.k8sObjects...), p, operatorNamespace, &tc.spec)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if tc.check != nil {
				tc.check(t, &tc.spec)
			}
		})
	}
}

func Test_substitute(t *testing.T) {
	vars := map[string][]byte{
		"A": []byte("alpha"),
		"B": []byte("beta"),
	}
	for _, tc := range []struct {
		input string
		want  string
	}{
		// basic substitution
		{"${A}", "alpha"},
		{"${B}", "beta"},
		// adjacent expressions with no separator
		{"${A}${B}", "alphabeta"},
		// unterminated ${ — left verbatim
		{"${A", "${A"},
		// empty expression name — not a known key, passed through verbatim
		{"${}", "${}"},
		// unknown variable without default — passed through verbatim
		{"${UNKNOWN}", "${UNKNOWN}"},
		// literal ${ not followed by a closing brace anywhere — left verbatim
		{"prefix ${ suffix", "prefix ${ suffix"},
		// text before and after
		{"before ${A} after", "before alpha after"},
		// no expressions — returned unchanged
		{"no expressions here", "no expressions here"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.want, string(substitute([]byte(tc.input), vars)))
		})
	}
}

func Test_indexClosingBrace(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  int
	}{
		{"VAR}", 3},
		{"VAR:-default}", 12},
		{"VAR:-{x}}", 8},       // nested brace in default
		{"VAR:-{{a}{b}}}", 13}, // multiple nested braces
		{"VAR:-}}", 5},         // } as default: closes at first }, same as bash
		{"VAR", -1},            // no closing brace
	} {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.want, indexClosingBrace([]byte(tc.input)))
		})
	}
}
