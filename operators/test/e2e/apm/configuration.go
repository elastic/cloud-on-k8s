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
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/stack"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type PartialApmConfiguration struct {
	Output struct {
		Elasticsearch struct {
			CompressionLevel int `yaml:"compression_level"`
		} `yaml:"elasticsearch"`
	} `yaml:"output"`
}

func AddConfigurationTestSteps(s Builder, k *helpers.K8sHelper) []helpers.TestStep {

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

	var previousPodUID *types.UID

	return helpers.TestStepList{}.
		WithSteps(

			// Check that we actually have a default value in the configuration
			helpers.TestStep{
				Name: "Check the value of a parameter in the configuration",
				Test: func(t *testing.T) {
					// get current pod id
					pods, err := k.GetPods(helpers.ApmServerPodListOptions(s.ApmServer.Name))
					require.NoError(t, err)
					require.True(t, len(pods) == 1)

					// exec into the pod to list keystore entries
					stdout, stderr, err := k.Exec(k8s.ExtractNamespacedName(&pods[0]), []string{"cat", "/usr/share/apm-server/config/config-secret/apm-server.yml"})
					if err != nil {
						require.NoError(t, errors.Wrap(err, fmt.Sprintf("stdout:\n%s\nstderr:\n%s", stdout, stderr)))
					}

					var config PartialApmConfiguration
					require.NoError(t, yaml.Unmarshal([]byte(stdout), &config))
					require.Equal(t, config.Output.Elasticsearch.CompressionLevel, 5) // 5 is the expected default value
				},
			},

			helpers.TestStep{
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
			stack.CheckKeystoreEntries(k, helpers.ApmServerPodListOptions(s.ApmServer.Name), stack.APMKeystoreCmd, nil),

			helpers.TestStep{
				Name: "Add a Keystore to the APM server",
				Test: func(t *testing.T) {
					// get current pod id
					pods, err := k.GetPods(helpers.ApmServerPodListOptions(s.ApmServer.Name))
					require.NoError(t, err)
					require.True(t, len(pods) == 1)
					previousPodUID = &pods[0].UID

					var apm apmtype.ApmServer
					require.NoError(t, k.Client.Get(k8s.ExtractNamespacedName(&s.ApmServer), &apm))
					apm.Spec.SecureSettings = &v1alpha1.SecretRef{
						SecretName: secureSettingsSecretName,
					}
					require.NoError(t, k.Client.Update(&apm))
				},
			},

			helpers.TestStep{
				Name: "APM Pod should be recreated",
				Test: helpers.Eventually(func() error {
					// get current pod id
					pods, err := k.GetPods(helpers.ApmServerPodListOptions(s.ApmServer.Name))
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

			// Check that the key is loaded in the APM keystore
			stack.CheckKeystoreEntries(k, helpers.ApmServerPodListOptions(s.ApmServer.Name), stack.APMKeystoreCmd, []string{"logging.verbose"}),

			helpers.TestStep{
				Name: "Customize configuration of the APM server",
				Test: func(t *testing.T) {
					// get current pod id
					pods, err := k.GetPods(helpers.ApmServerPodListOptions(s.ApmServer.Name))
					require.NoError(t, err)
					require.True(t, len(pods) == 1)
					previousPodUID = &pods[0].UID

					var apm apmtype.ApmServer
					require.NoError(t, k.Client.Get(k8s.ExtractNamespacedName(&s.ApmServer), &apm))
					customConfig := commonv1alpha1.Config{
						Data: map[string]interface{}{"output.elasticsearch.compression_level": 1},
					}
					apm.Spec.Config = &customConfig
					require.NoError(t, k.Client.Update(&apm))
				},
			},

			helpers.TestStep{
				Name: "APM Pod should be recreated",
				Test: helpers.Eventually(func() error {
					// get current pod id
					pods, err := k.GetPods(helpers.ApmServerPodListOptions(s.ApmServer.Name))
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

			helpers.TestStep{
				Name: "Check the value of a parameter in the configuration",
				Test: func(t *testing.T) {
					// get current pod id
					pods, err := k.GetPods(helpers.ApmServerPodListOptions(s.ApmServer.Name))
					require.NoError(t, err)
					require.True(t, len(pods) == 1)

					// exec into the pod to list keystore entries
					stdout, stderr, err := k.Exec(k8s.ExtractNamespacedName(&pods[0]), []string{"cat", "/usr/share/apm-server/config/config-secret/apm-server.yml"})
					if err != nil {
						require.NoError(t, errors.Wrap(err, fmt.Sprintf("stdout:\n%s\nstderr:\n%s", stdout, stderr)))
					}

					var config PartialApmConfiguration
					require.NoError(t, yaml.Unmarshal([]byte(stdout), &config))
					require.Equal(t, config.Output.Elasticsearch.CompressionLevel, 1) // 5 is the expected default value
				},
			},

			helpers.TestStep{
				Name: "Delete secure settings secret",
				Test: func(t *testing.T) {
					err := k.Client.Delete(&secureSettings)
					require.NoError(t, err)
				},
			},
		)
}
