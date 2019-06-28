// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"testing"

	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework/elasticsearch"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUpdateESSecureSettings(t *testing.T) {
	k := framework.NewK8sClientOrFatal()

	// user-provided secure settings secret
	secureSettingsSecretName := "secure-settings-secret"
	secureSettings := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secureSettingsSecretName,
			Namespace: framework.Namespace,
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
	b := elasticsearch.NewBuilder("test-es-keystore").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithESSecureSettings(secureSettings.Name)

	framework.TestStepList{}.
		// create secure settings secret
		WithStep(framework.TestStep{
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
		WithSteps(b.InitTestSteps(k)).
		WithSteps(b.CreationTestSteps(k)).
		WithSteps(framework.CheckTestSteps(b, k)).
		WithSteps(framework.TestStepList{
			// initial secure settings should be there in all nodes keystore
			elasticsearch.CheckESKeystoreEntries(k, b.Elasticsearch, []string{
				"file.setting1", "file.setting2", "key.without.prefix", "string.setting1", "string.setting2"}),

			// modify the secure settings secret
			framework.TestStep{
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
			elasticsearch.CheckESKeystoreEntries(k, b.Elasticsearch, []string{
				"new.file.setting", "new.string.setting2", "string.setting1"}),

			// remove the secure settings reference
			framework.TestStep{
				Name: "Remove secure settings from the spec",
				Test: func(t *testing.T) {
					// retrieve current Elasticsearch resource
					var currentEs estype.Elasticsearch
					err := k.Client.Get(k8s.ExtractNamespacedName(&b.Elasticsearch), &currentEs)
					require.NoError(t, err)
					// set its secure settings to nil
					currentEs.Spec.SecureSettings = nil
					err = k.Client.Update(&currentEs)
					require.NoError(t, err)
				},
			},

			// keystore should be updated accordingly
			elasticsearch.CheckESKeystoreEntries(k, b.Elasticsearch, nil),

			// cleanup extra resources
			framework.TestStep{
				Name: "Delete secure settings secret",
				Test: func(t *testing.T) {
					err := k.Client.Delete(&secureSettings)
					require.NoError(t, err)
				},
			},
		}).
		WithSteps(b.DeletionTestSteps(k)).
		RunSequential(t)
}
