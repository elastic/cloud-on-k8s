// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/stack"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUpdateESSecureSettings(t *testing.T) {
	k := helpers.NewK8sClientOrFatal()

	// user-provided secure settings secret
	secureSettingsSecretName := "secure-settings-secret"
	secureSettings := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secureSettingsSecretName,
			Namespace: params.Namespace,
		},
		Data: map[string][]byte{
			"key.without.prefix": []byte("string value"),
			"string.setting1":    []byte("string value"),
			"string.setting2":    []byte("string value"),
			"file.setting1":      []byte("file content"),
			"file.setting2":      []byte("file content"),
		},
	}

	// set up a 3-nodes cluster with secure settings
	s := stack.NewStackBuilder("test-keystore").
		WithESMasterDataNodes(3, stack.DefaultResources).
		WithESSecureSettings(secureSettings.Name)

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
			stack.CheckESKeystoreEntries(k, s.Elasticsearch, []string{
				"file.setting1", "file.setting2", "key.without.prefix", "string.setting1", "string.setting2"}),

			// modify the secure settings secret
			helpers.TestStep{
				Name: "Modify secure settings secret",
				Test: func(t *testing.T) {
					// remove some keys, add new ones
					secureSettings.Data = map[string][]byte{
						"string.setting1":     []byte("new string content"), // the actual value update cannot be checked :(
						"new.string.setting2": []byte("string content"),
						"new.file.setting":    []byte("file content"),
					}
					err := k.Client.Update(&secureSettings)
					require.NoError(t, err)
				},
			},

			// keystore should be updated accordingly
			stack.CheckESKeystoreEntries(k, s.Elasticsearch, []string{
				"new.file.setting", "new.string.setting2", "string.setting1"}),

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
			stack.CheckESKeystoreEntries(k, s.Elasticsearch, nil),

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

func TestUpdateKibanaSecureSettings(t *testing.T) {
	k := helpers.NewK8sClientOrFatal()

	// user-provided secure settings secret
	secureSettingsSecretName := "secure-settings-secret"
	secureSettings := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secureSettingsSecretName,
			Namespace: params.Namespace,
		},
		Data: map[string][]byte{
			// this needs to be a valid configuration item, otherwise Kibana refuses to start
			"logging.verbose": []byte("true"),
		},
	}

	// set up a 1-node Kibana deployment with secure settings
	s := stack.NewStackBuilder("test-keystore").
		WithESMasterDataNodes(1, stack.DefaultResources).
		WithKibana(1).
		WithKibanaSecureSettings(secureSettings.Name)

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

			stack.CheckKibanaKeystoreEntries(k, s.Kibana, []string{"logging.verbose"}),

			// modify the secure settings secret
			helpers.TestStep{
				Name: "Modify secure settings secret",
				Test: func(t *testing.T) {
					secureSettings.Data = map[string][]byte{
						// this needs to be a valid configuration item, otherwise Kibana refuses to start
						"logging.json":    []byte("true"),
						"logging.verbose": []byte("true"),
					}
					err := k.Client.Update(&secureSettings)
					require.NoError(t, err)
				},
			},

			// keystore should be updated accordingly
			stack.CheckKibanaKeystoreEntries(k, s.Kibana, []string{"logging.json", "logging.verbose"}),

			// remove the secure settings reference
			helpers.TestStep{
				Name: "Remove secure settings from the spec",
				Test: func(t *testing.T) {
					// retrieve current resource version
					var kb kbtype.Kibana
					err := k.Client.Get(k8s.ExtractNamespacedName(&s.Kibana), &kb)
					require.NoError(t, err)
					// set its secure settings to nil
					kb.Spec.SecureSettings = nil
					err = k.Client.Update(&kb)
					require.NoError(t, err)
				},
			},

			// keystore should be updated accordingly
			stack.CheckKibanaKeystoreEntries(k, s.Kibana, nil),

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
