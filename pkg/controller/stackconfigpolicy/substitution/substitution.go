// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package substitution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// Apply resolves variables from policy.Spec.VariablesFrom and substitutes them into spec in place.
// It is a no-op when VariablesFrom is empty.
// operatorNamespace is used to enforce the cross-namespace scoping rule at read time: a
// namespace-scoped policy may not read sources outside its own namespace even when the validating
// webhook is bypassed.
func Apply[T any](ctx context.Context, client k8s.Client, policy *policyv1alpha1.StackConfigPolicy, operatorNamespace string, spec *T) error {
	if len(policy.Spec.VariablesFrom) == 0 {
		return nil
	}
	vars, err := resolveVars(ctx, client, policy, operatorNamespace)
	if err != nil {
		return err
	}
	return substituteVars(spec, vars)
}

// indexClosingBrace returns the index of the } that closes the ${ already consumed,
// correctly handling nested { } pairs. Returns -1 if no matching } is found.
func indexClosingBrace(s []byte) int {
	depth := 1
	for i, r := range s {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// jsonEscape encodes v as a JSON string value (without surrounding quotes), making it safe
// to substitute directly into a JSON string literal. This ensures that values containing
// backslashes, quotes, newlines (e.g. PEM certificates, Windows paths) produce valid JSON.
func jsonEscape(v string) []byte {
	b, _ := json.Marshal(v)
	// json.Marshal always succeeds for strings; strip the surrounding quotes.
	return b[1 : len(b)-1]
}

// resolveVars builds the substitution variable map from VariablesFrom sources.
// Later entries take precedence over earlier ones on key conflicts.
// Values are JSON-escaped so they can be substituted safely into a JSON string literal.
// The cross-namespace scoping rule is enforced here so the read path is self-guarding even
// when the validating webhook is bypassed (e.g. failurePolicy=ignore) and this policy is
// pulled into a sibling policy's merge.
func resolveVars(ctx context.Context, client k8s.Client, policy *policyv1alpha1.StackConfigPolicy, operatorNamespace string) (map[string][]byte, error) {
	pNsn := k8s.ExtractNamespacedName(policy)
	vars := make(map[string][]byte)
	for idx, src := range policy.Spec.VariablesFrom {
		srcNamespace := src.EffectiveNamespace(policy.Namespace)
		if !src.AllowedFrom(policy.Namespace, operatorNamespace) {
			return nil, fmt.Errorf("policy %q may not reference variablesFrom source %q in namespace %q",
				pNsn, src.Name, srcNamespace)
		}
		nsn := types.NamespacedName{Namespace: srcNamespace, Name: src.Name}
		switch src.Kind {
		case policyv1alpha1.VariableSourceKindConfigMap:
			var cm corev1.ConfigMap
			if err := client.Get(ctx, nsn, &cm); err != nil {
				if src.Optional && apierrors.IsNotFound(err) {
					continue
				}
				return nil, fmt.Errorf("error reading ConfigMap %q for %q: %w", nsn, pNsn, err)
			}
			for k, v := range cm.Data {
				vars[k] = jsonEscape(v)
			}
		case policyv1alpha1.VariableSourceKindSecret:
			var secret corev1.Secret
			if err := client.Get(ctx, nsn, &secret); err != nil {
				if src.Optional && apierrors.IsNotFound(err) {
					continue
				}
				return nil, fmt.Errorf("error reading Secret %q for %q: %w", nsn, pNsn, err)
			}
			for k, v := range secret.Data {
				vars[k] = jsonEscape(string(v))
			}
		default:
			// The CRD enum marker (+kubebuilder:validation:Enum=ConfigMap;Secret) makes any other
			// kind unreachable at the API level; this branch is a defensive guard against direct
			// object creation that bypasses the API server.
			return nil, fmt.Errorf("unknown variablesFrom kind %q for %q at index %d", src.Kind, pNsn, idx)
		}
	}
	return vars, nil
}

// substituteVars marshals spec to JSON, applies substitute, then unmarshals back in place.
func substituteVars[T any](spec *T, vars map[string][]byte) error {
	data, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("failed to marshal spec: %w", err)
	}
	substituted := substitute(data, vars)
	if err := json.Unmarshal(substituted, spec); err != nil {
		return fmt.Errorf("failed to unmarshal substituted spec: %w", err)
	}
	return nil
}

// substitute replaces ${VAR} and ${VAR:-default} expressions in a string.
// Only keys present in vars are substituted; all other ${...} expressions are left verbatim
// so that native ES references (e.g. ${ENV}, ${foo.bar}) survive unchanged.
// The :- separator (bash-style) marks ECK defaults and is intentionally distinct from
// ES's ${name:value} single-colon syntax, which therefore always passes through.
// Values sourced from ConfigMaps/Secrets are pre-escaped by resolveVars; inline defaults
// are part of the policy spec and were JSON-encoded during marshal, so they are already safe.
func substitute(data []byte, vars map[string][]byte) []byte {
	var (
		startToken = []byte("${")
		defaultVal = []byte(":-")
	)
	var buf bytes.Buffer
	for {
		start := bytes.Index(data, startToken)
		if start == -1 {
			buf.Write(data)
			break
		}
		buf.Write(data[:start])
		data = data[start+2:] // consume "${"
		end := indexClosingBrace(data)
		if end == -1 {
			// no closing brace — restore the "${" we consumed and leave the rest as-is
			buf.WriteString("${")
			buf.Write(data)
			break
		}
		expr := data[:end]
		data = data[end+1:] // consume expression and "}"

		name, def, hasDef := bytes.Cut(expr, defaultVal)
		val, defined := vars[string(name)]
		switch {
		case defined && (len(val) > 0 || !hasDef):
			buf.Write(val)
		case hasDef:
			buf.Write(def)
		default:
			buf.WriteString("${") // undefined with no default — pass through verbatim
			buf.Write(expr)
			buf.WriteString("}")
		}
	}
	return buf.Bytes()
}
