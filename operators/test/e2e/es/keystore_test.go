// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"testing"

	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/elasticsearch"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUpdateESSecureSettings(t *testing.T) {
	k := test.NewK8sClientOrFatal()

	// user-provided secure settings secret
	secureSettingsSecretName := "secure-settings-secret"
	secureSettings := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secureSettingsSecretName,
			Namespace: test.Namespace,
		},
		Data: map[string][]byte{
			// this needs to be a valid configuration item, otherwise Kibana refuses to start
			"path.logs":                    []byte("/tmp/logs"),
			"xpack.security.audit.enabled": []byte("false"),
		},
	}

	// set up a 3-nodes cluster with secure settings
	b := elasticsearch.NewBuilder("test-es-keystore").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithESSecureSettings(secureSettings.Name)

	test.StepList{}.
		// create secure settings secret
		WithStep(test.Step{
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
		WithSteps(test.CheckTestSteps(b, k)).
		WithSteps(test.StepList{
			// initial secure settings should be there in all nodes keystore
			elasticsearch.CheckESKeystoreEntries(k, b.Elasticsearch, []string{
				"path.logs", "xpack.security.audit.enabled"}),

			// modify the secure settings secret
			test.Step{
				Name: "Modify secure settings secret",
				Test: func(t *testing.T) {
					// remove some keys, add new ones
					secureSettings.Data = map[string][]byte{
						"path.logs": []byte("/tmp/logs2"), // the actual value update cannot be checked :(
					}
					err := k.Client.Update(&secureSettings)
					require.NoError(t, err)
				},
			},

			// keystore should be updated accordingly
			elasticsearch.CheckESKeystoreEntries(k, b.Elasticsearch, []string{
				"path.logs"}),

			// remove the secure settings reference
			test.Step{
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
			test.Step{
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
