// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-test/deep"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/about"
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
	minExpectedKeys := len(e.Keys)
	maxExpectedKeys := minExpectedKeys + len(e.OptionalKeys)
	if len(s.Data) < minExpectedKeys || maxExpectedKeys < len(s.Data) {
		return fmt.Errorf("expected between %d and %d keys in %s, got %d", minExpectedKeys, maxExpectedKeys, e.Name, len(s.Data))
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

// CheckSpecNotOwnedByOperator verifies that the ECK operator has not claimed ownership of the
// resource spec in managed fields.
func CheckSpecNotOwnedByOperator(obj k8sclient.Object, k *K8sClient) Step {
	return Step{
		Name: "Spec should not be owned by the operator in managed fields",
		Test: Eventually(func() error {
			// Do not mutate the caller's object.
			live, ok := obj.DeepCopyObject().(k8sclient.Object)
			if !ok {
				return fmt.Errorf("expected object to be of type client.Object, got %T", live)
			}
			if err := k.Client.Get(context.Background(), types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, live); err != nil {
				return err
			}
			for _, entry := range live.GetManagedFields() {
				if entry.Subresource != "" || entry.FieldsV1 == nil {
					continue
				}
				if entry.Manager != about.FieldOwner {
					continue
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(entry.FieldsV1.GetRawBytes(), &fields); err != nil {
					return fmt.Errorf("parsing managed fields for manager %q: %w", entry.Manager, err)
				}
				if _, ownsSpec := fields["f:spec"]; ownsSpec {
					return fmt.Errorf("field manager %q owns f:spec: the operator should not claim spec ownership", entry.Manager)
				}
			}
			return nil
		}),
	}
}

func CheckSelector(actualSelector string, expectedLabels map[string]string) error {
	labelSelector, err := v1.ParseToLabelSelector(actualSelector)
	if err != nil {
		return err
	}
	if diff := deep.Equal(expectedLabels, labelSelector.MatchLabels); diff != nil {
		return errors.New(strings.Join(diff, ", "))
	}
	return nil
}
