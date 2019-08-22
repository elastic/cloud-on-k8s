// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kb

import (
	"testing"

	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/kibana"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	KibanaKeystoreBin = "/usr/share/kibana/bin/kibana-keystore"
)

var KibanaKeystoreCmd = []string{KibanaKeystoreBin}

func TestUpdateKibanaSecureSettings(t *testing.T) {
	// user-provided secure settings secret
	secureSettingsSecretName := "secure-settings-secret"
	secureSettings := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secureSettingsSecretName,
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Data: map[string][]byte{
			// this needs to be a valid configuration item, otherwise Kibana refuses to start
			"logging.verbose": []byte("true"),
		},
	}

	// set up a 1-node Kibana deployment with secure settings
	name := "test-kb-keystore"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1).
		WithKibanaSecureSettings(secureSettings.Name)

	kbPodListOpts := test.KibanaPodListOptions(kbBuilder.Kibana.Namespace, kbBuilder.Kibana.Name)

	initStepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Create secure settings secret",
				Test: func(t *testing.T) {
					// remove if already exists (ignoring errors)
					_ = k.Client.Delete(&secureSettings)
					// and create a fresh one
					err := k.Client.Create(&secureSettings)
					require.NoError(t, err)
				},
			},
		}
	}

	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			test.CheckKeystoreEntries(k, kbPodListOpts, KibanaKeystoreCmd, []string{"logging.verbose"}),
			// modify the secure settings secret
			test.Step{
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
			test.CheckKeystoreEntries(k, kbPodListOpts, KibanaKeystoreCmd, []string{"logging.json", "logging.verbose"}),

			// remove the secure settings reference
			test.Step{
				Name: "Remove secure settings from the spec",
				Test: func(t *testing.T) {
					// retrieve current Kibana resource
					var currentKb kbtype.Kibana
					err := k.Client.Get(k8s.ExtractNamespacedName(&kbBuilder.Kibana), &currentKb)
					require.NoError(t, err)
					// set its secure settings to nil
					currentKb.Spec.SecureSettings = nil
					err = k.Client.Update(&currentKb)
					require.NoError(t, err)
				},
			},

			// keystore should be updated accordingly
			test.CheckKeystoreEntries(k, kbPodListOpts, KibanaKeystoreCmd, nil),

			// cleanup extra resources
			test.Step{
				Name: "Delete secure settings secret",
				Test: func(t *testing.T) {
					err := k.Client.Delete(&secureSettings)
					require.NoError(t, err)
				},
			},
		}
	}

	test.Sequence(initStepsFn, stepsFn, esBuilder, kbBuilder).RunSequential(t)
}
