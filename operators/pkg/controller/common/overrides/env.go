// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package overrides

import (
	corev1 "k8s.io/api/core/v1"
)

// EnvBuilder helps with building a list of environment variables while
// dealing with name conflicts.
type EnvBuilder struct {
	byName  map[string]struct{} // indicates existence
	envVars []corev1.EnvVar     // contains the actual value, preserving order
}

// NewEnvBuilder returns an EnvBuilder initialized with the given vars.
func NewEnvBuilder(vars ...corev1.EnvVar) *EnvBuilder {
	byName := make(map[string]struct{}, len(vars))
	envVars := make([]corev1.EnvVar, 0, len(vars))
	envBuilder := EnvBuilder{byName: byName, envVars: envVars}
	envBuilder.AddOrOverride(vars...)
	return &envBuilder
}

// GetEnvVars returns a list of unique environment variables.
func (e *EnvBuilder) GetEnvVars() []corev1.EnvVar {
	return e.envVars
}

// AddIfMissing adds the given variables if variables with the same name don't already exist.
func (e *EnvBuilder) AddIfMissing(vars ...corev1.EnvVar) {
	for _, v := range vars {
		if _, exists := e.byName[v.Name]; exists {
			// a variable with the same name already exists, keep the existing one
			continue
		}
		e.byName[v.Name] = struct{}{}
		e.envVars = append(e.envVars, v)
	}
}

// replace the given newVar in the list of existing ones.
func replace(existing []corev1.EnvVar, newVar corev1.EnvVar) []corev1.EnvVar {
	for i, v := range existing {
		if v.Name == newVar.Name {
			// replace existing one and return early
			existing[i] = newVar
			return existing
		}
	}
	return existing
}

// AddOrOverride adds the given variables, overriding values of existing ones that may already exist.
func (e *EnvBuilder) AddOrOverride(vars ...corev1.EnvVar) {
	for _, v := range vars {
		if _, exists := e.byName[v.Name]; exists {
			e.envVars = replace(e.envVars, v)
		} else {
			e.envVars = append(e.envVars, v)
		}
		e.byName[v.Name] = struct{}{}
	}
}
