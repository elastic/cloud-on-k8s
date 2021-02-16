// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// CheckTestSteps returns all test steps to verify a given resource in K8s is the expected one
// and the given resource is running as expected.
func CheckTestSteps(b Builder, k *K8sClient) StepList {
	return StepList{}.
		WithSteps(b.CheckK8sTestSteps(k)).
		WithSteps(b.CheckStackTestSteps(k))
}

// ExpectedSecret represents a Secret we expect to exist.
type ExpectedSecret struct {
	Name         string
	Labels       map[string]string
	Keys         []string
	OptionalKeys []string
}

// MatchesActualSecret fetches the corresponding secret from k and returns an error if it mismatches.
func (e ExpectedSecret) MatchesActualSecret(k *K8sClient, namespace string) error {
	// secret should exist
	var s corev1.Secret
	if err := k.Client.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: e.Name}, &s); err != nil {
		return err
	}

	// have the expected keys
	min := len(e.Keys)
	max := min + len(e.OptionalKeys)
	if len(s.Data) < min || max < len(s.Data) {
		return fmt.Errorf("expected between %d and %d keys in %s, got %d", min, max, e.Name, len(s.Data))
	}
	for _, k := range e.Keys {
		if _, exists := s.Data[k]; !exists {
			return fmt.Errorf("expected key %s in secret %s not found", k, e.Name)
		}
	}
	// and labels (actual secret can have more labels)
	for k, v := range e.Labels {
		actualValue, exists := s.Labels[k]
		if !exists {
			return fmt.Errorf("expected label %s not found in %s", k, e.Name)
		}
		if actualValue != v {
			return fmt.Errorf("expected value %s for label %s in secret %s, found %s", v, k, e.Name, actualValue)
		}
	}
	return nil
}

// CheckSecretsContent checks that expected secrets exist.
func CheckSecretsContent(k *K8sClient, namespace string, expected func() []ExpectedSecret) Step {
	return Step{
		Name: "Secrets should eventually be created",
		Test: Eventually(func() error {
			for _, e := range expected() {
				if err := e.MatchesActualSecret(k, namespace); err != nil {
					return err
				}
			}
			return nil
		}),
	}
}
