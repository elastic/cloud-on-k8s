// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package substitution

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fluxcd/pkg/envsubst"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// jsonEscape returns s safe to splice into a JSON string literal.
func jsonEscape(s string) string {
	b, _ := json.Marshal(s) // json.Marshal never fails for plain strings
	return string(b[1 : len(b)-1])
}

// ResolveVars builds the substitution variable map from VariablesFrom sources.
// Later entries in VariablesFrom take precedence over earlier ones on key conflicts.
// Values are JSON-escaped so they are safe to splice into a JSON string literal.
func ResolveVars(ctx context.Context, client k8s.Client, policy *policyv1alpha1.StackConfigPolicy) (map[string]string, error) {
	if policy == nil || len(policy.Spec.VariablesFrom) == 0 {
		return nil, nil
	}

	pNsn := k8s.ExtractNamespacedName(policy)
	vars := make(map[string]string)
	for idx, src := range policy.Spec.VariablesFrom {
		nsn := types.NamespacedName{Namespace: src.EffectiveNamespace(policy.Namespace), Name: src.Name}
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
			return nil, fmt.Errorf("unknown variablesFrom kind %q for %q at index %d", src.Kind, pNsn, idx)
		}
	}
	return vars, nil
}

// SubstituteVars marshals spec to JSON, substitutes ${VAR} references, and unmarshals back in place.
func SubstituteVars[T any](spec *T, vars map[string]string) error {
	data, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("failed to marshal spec: %w", err)
	}
	substituted, err := envsubst.Eval(string(data), func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	})
	if err != nil {
		return fmt.Errorf("failed to substitute vars: %w", err)
	}
	if err := json.Unmarshal([]byte(substituted), spec); err != nil {
		return fmt.Errorf("failed to unmarshal substituted spec: %w", err)
	}
	return nil
}
