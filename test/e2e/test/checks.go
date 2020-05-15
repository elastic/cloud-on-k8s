// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
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

// CheckSecretsContent checks that expected secrets exist.
// expected() returns a map[<secret name>] -> {<secret keys>}.
func CheckSecretsContent(k *K8sClient, namespace string, expected func() map[string][]string) Step {
	return Step{
		Name: "Secrets should eventually be created",
		Test: Eventually(func() error {
			for secretName, keys := range expected() {
				// secret should exist
				var s corev1.Secret
				if err := k.Client.Get(types.NamespacedName{Namespace: namespace, Name: secretName}, &s); err != nil {
					return err
				}
				// and have the expected keys
				if len(s.Data) != len(keys) {
					return fmt.Errorf("expected %d keys in %s, got %d", len(keys), secretName, len(s.Data))
				}
				for _, k := range keys {
					if _, exists := s.Data[k]; !exists {
						return fmt.Errorf("expected key %s in secret %s not found", k, secretName)
					}
				}
			}
			return nil
		}),
	}
}
