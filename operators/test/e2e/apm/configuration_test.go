// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apm

import (
	"fmt"
	"testing"

	apmtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/elasticsearch"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	APMKeystoreBin    = "/usr/share/apm-server/apm-server"
	APMKeystoreOption = "keystore"
)

var APMKeystoreCmd = []string{APMKeystoreBin, APMKeystoreOption}

type PartialApmConfiguration struct {
	Output struct {
		Elasticsearch struct {
			CompressionLevel int `yaml:"compression_level"`
		} `yaml:"elasticsearch"`
	} `yaml:"output"`
}

func TestUpdateConfiguration(t *testing.T) {

	// user-provided secure settings secret
	secureSettingsSecretName := "secure-settings-secret"
	secureSettings := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secureSettingsSecretName,
			Namespace: test.Namespace,
		},
		Data: map[string][]byte{
			"logging.verbose": []byte("true"),
		},
	}

	name := "test-apm-configuration"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	apmBuilder := apmserver.NewBuilder(name).
		WithNamespace(test.Namespace).
		WithVersion(test.ElasticStackVersion).
		WithRestrictedSecurityContext()

	var previousPodUID *types.UID

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
			// Keystore should be empty
			test.CheckKeystoreEntries(k, test.ApmServerPodListOptions(name), APMKeystoreCmd, nil),
		}
	}
	apmNamespacedName := types.NamespacedName{
		Name:      name,
		Namespace: test.Namespace,
	}

	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Check the value of a parameter in the configuration",
				Test: func(t *testing.T) {
					config, err := partialAPMConfiguration(k, name)
					require.NoError(t, err)
					require.Equal(t, config.Output.Elasticsearch.CompressionLevel, 5) // 5 is the expected default value
				},
			},
			test.Step{
				Name: "Add a Keystore to the APM server",
				Test: func(t *testing.T) {
					// get current pod id
					pods, err := k.GetPods(test.ApmServerPodListOptions(name))
					require.NoError(t, err)
					require.True(t, len(pods) == 1)
					previousPodUID = &pods[0].UID

					var apm apmtype.ApmServer
					require.NoError(t, k.Client.Get(apmNamespacedName, &apm))
					apm.Spec.SecureSettings = &v1alpha1.SecretRef{
						SecretName: secureSettingsSecretName,
					}
					require.NoError(t, k.Client.Update(&apm))
				},
			},
			test.Step{
				Name: "APM Pod should be recreated",
				Test: test.Eventually(func() error {
					// get current pod id
					pods, err := k.GetPods(test.ApmServerPodListOptions(name))
					if err != nil {
						return err
					}
					if len(pods) != 1 {
						return fmt.Errorf("1 APM pod expected, got %d", len(pods))
					}
					if pods[0].UID == *previousPodUID {
						return fmt.Errorf("APM pod is still the same, uid: %s", pods[0].UID)
					}
					return nil
				}),
			},

			test.CheckKeystoreEntries(k, test.ApmServerPodListOptions(name), APMKeystoreCmd, []string{"logging.verbose"}),

			test.Step{
				Name: "Customize configuration of the APM server",
				Test: func(t *testing.T) {
					// get current pod id
					pods, err := k.GetPods(test.ApmServerPodListOptions(name))
					require.NoError(t, err)
					require.True(t, len(pods) == 1)
					previousPodUID = &pods[0].UID

					var apm apmtype.ApmServer
					require.NoError(t, k.Client.Get(apmNamespacedName, &apm))
					customConfig := commonv1alpha1.Config{
						Data: map[string]interface{}{"output.elasticsearch.compression_level": 1},
					}
					apm.Spec.Config = &customConfig
					require.NoError(t, k.Client.Update(&apm))
				},
			},
			test.Step{
				Name: "APM Pod should be recreated",
				Test: test.Eventually(func() error {
					// get current pod id
					pods, err := k.GetPods(test.ApmServerPodListOptions(name))
					if err != nil {
						return err
					}
					if len(pods) != 1 {
						return fmt.Errorf("1 APM pod expected, got %d", len(pods))
					}
					if pods[0].UID == *previousPodUID {
						return fmt.Errorf("APM pod is still the same, uid: %s", pods[0].UID)
					}
					return nil
				}),
			},

			test.Step{
				Name: "Check the value of a parameter in the configuration",
				Test: func(t *testing.T) {
					config, err := partialAPMConfiguration(k, name)
					require.NoError(t, err)
					require.Equal(t, config.Output.Elasticsearch.CompressionLevel, 1) // value should be updated to 1
				},
			},

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

	test.Sequence(initStepsFn, stepsFn, esBuilder, apmBuilder).RunSequential(t)

}

func partialAPMConfiguration(k *test.K8sClient, name string) (PartialApmConfiguration, error) {
	var config PartialApmConfiguration
	// get current pod id
	pods, err := k.GetPods(test.ApmServerPodListOptions(name))
	if err != nil {
		return config, err
	}
	// exec into the pod to list keystore entries
	stdout, stderr, err := k.Exec(k8s.ExtractNamespacedName(&pods[0]), []string{"cat", "/usr/share/apm-server/config/config-secret/apm-server.yml"})
	if err != nil {
		return config, errors.Wrap(err, fmt.Sprintf("stdout:\n%s\nstderr:\n%s", stdout, stderr))
	}
	err = yaml.Unmarshal([]byte(stdout), &config)
	if err != nil {
		return config, err
	}
	return config, nil
}
