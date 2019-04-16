// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	"github.com/elastic/k8s-operators/operators/test/e2e/stack"
)

func TestUpdateSecureSettings(t *testing.T) {
	k := helpers.NewK8sClientOrFatal()

	// user-provided secure settings secret
	secureSettingsSecretName := "secure-settings-secret"
	secureSettings := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secureSettingsSecretName,
			Namespace: helpers.DefaultNamespace,
		},
		Data: map[string][]byte{
			"key1": []byte("initialValue1"),
			"key2": []byte("initialValue2"),
		},
	}

	// set up a 3-nodes cluster with secure settings
	s := stack.NewStackBuilder("test-keystore").
		WithESMasterDataNodes(3, stack.DefaultResources).
		WithSecureSettings(secureSettings.Name)

	helpers.TestStepList{}.
		// create secure settings secret
		WithSteps(helpers.TestStep{
			Name: "Create secure settings secret",
			Test: func(t *testing.T) {
				// remove if already exists (ignoring errors)
				_ = k.Client.Delete(&secureSettings)
				// and create a fresh one
				err := k.Client.Create(&secureSettings)
				require.NoError(t, err)
			},
		}).

		// create the cluster
		WithSteps(stack.InitTestSteps(s, k)...).
		WithSteps(stack.CreationTestSteps(s, k)...).
		WithSteps(

			// initial secure settings should be there in all nodes keystore
			stack.CheckKeystoreEntries(k, s.Elasticsearch, []string{"key1", "key2"}),

			// modify the secure settings secret
			helpers.TestStep{
				Name: "Modify secure settings secret",
				Test: func(t *testing.T) {
					// remove key2, add key3
					secureSettings.Data = map[string][]byte{
						"key1": []byte("updatedValue1"), // the actual value update cannot be checked :(
						"key3": []byte("initialValue3"),
					}
					err := k.Client.Update(&secureSettings)
					require.NoError(t, err)
				},
			},

			// keystore should be updated accordingly
			stack.CheckKeystoreEntries(k, s.Elasticsearch, []string{"key1", "key3"}),

			// remove the secure settings reference
			helpers.TestStep{
				Name: "Remove secure settings from the spec",
				Test: func(t *testing.T) {
					// retrieve current resource version
					var es v1alpha1.Elasticsearch
					err := k.Client.Get(k8s.ExtractNamespacedName(&s.Elasticsearch), &es)
					require.NoError(t, err)
					// set its secure settings to nil
					es.Spec.SecureSettings = nil
					err = k.Client.Update(&es)
					require.NoError(t, err)
				},
			},

			// keystore should be updated accordingly
			stack.CheckKeystoreEntries(k, s.Elasticsearch, nil),

			// cleanup extra resources
			helpers.TestStep{
				Name: "Delete secure settings secret",
				Test: func(t *testing.T) {
					err := k.Client.Delete(&secureSettings)
					require.NoError(t, err)
				},
			},
		).
		RunSequential(t)
}
