// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build apm || e2e

package apm

import (
	"context"
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
)

const (
	APMKeystoreBin    = "/usr/share/apm-server/apm-server"
	APMKeystoreOption = "keystore"
)

var APMKeystoreCmd = []string{APMKeystoreBin, APMKeystoreOption}

type PartialApmConfiguration struct {
	Output struct {
		Elasticsearch struct {
			Hosts            []string `yaml:"hosts"`
			CompressionLevel int      `yaml:"compression_level"`
		} `yaml:"elasticsearch"`
	} `yaml:"output"`
}

func TestUpdateConfiguration(t *testing.T) {

	// user-provided secure settings secret
	secureSettingsSecretName := "secure-settings-secret"
	secureSettings := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secureSettingsSecretName,
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Data: map[string][]byte{
			"logging.verbose": []byte("true"),
		},
	}

	name := "test-apm-configuration"
	namespace := test.Ctx().ManagedNamespace(0)
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)
	apmBuilder := apmserver.NewBuilder(name).
		WithNamespace(namespace).
		WithElasticsearchRef(esBuilder.Ref()).
		WithVersion(test.Ctx().ElasticStackVersion).
		WithRestrictedSecurityContext().
		WithoutIntegrationCheck()

	apmName := apmBuilder.ApmServer.Name
	apmNamespacedName := types.NamespacedName{
		Name:      apmName,
		Namespace: namespace,
	}
	apmPodListOpts := test.ApmServerPodListOptions(namespace, apmName)

	initStepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Create secure settings secret",
				Test: test.Eventually(func() error {
					return k.CreateOrUpdateSecrets(secureSettings)
				}),
			},
			// Keystore should be empty
			test.CheckKeystoreEntries(k, APMKeystoreCmd, nil, apmPodListOpts...),
		}
	}

	var previousPodUID *types.UID
	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Check the value of a parameter in the configuration",
				Test: test.Eventually(func() error {
					config, err := partialAPMConfiguration(k, namespace, apmName)
					if err != nil {
						return err
					}

					esHost := services.ExternalServiceURL(esBuilder.Elasticsearch)
					if config.Output.Elasticsearch.Hosts[0] != esHost {
						return fmt.Errorf("expected es host %s but got %s", esHost, config.Output.Elasticsearch.Hosts[0])
					}
					if config.Output.Elasticsearch.CompressionLevel != 0 { // CompressionLevel is not set by default
						return fmt.Errorf("expected compression level 0 but got %d", config.Output.Elasticsearch.CompressionLevel)
					}
					return nil
				}),
			},
			test.Step{
				Name: "Add a Keystore to the APM server",
				Test: test.Eventually(func() error {
					// get current pod id
					pods, err := k.GetPods(apmPodListOpts...)
					if err != nil {
						return err
					}
					if len(pods) != 1 {
						return fmt.Errorf("1 APM pod expected, got %d", len(pods))
					}
					previousPodUID = &pods[0].UID

					var apm apmv1.ApmServer
					if err := k.Client.Get(context.Background(), apmNamespacedName, &apm); err != nil {
						return err
					}
					apm.Spec.SecureSettings = []commonv1.SecretSource{
						{SecretName: secureSettingsSecretName},
					}
					return k.Client.Update(context.Background(), &apm)
				}),
			},
			test.Step{
				Name: "APM Pod should be recreated",
				Test: test.Eventually(func() error {
					// get current pod id
					pods, err := k.GetPods(apmPodListOpts...)
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

			test.CheckKeystoreEntries(k, APMKeystoreCmd, []string{"logging.verbose"}, apmPodListOpts...),

			test.Step{
				Name: "Customize configuration of the APM server",
				Test: test.Eventually(func() error {
					// get current pod id
					pods, err := k.GetPods(apmPodListOpts...)
					if err != nil {
						return err
					}
					if len(pods) != 1 {
						return fmt.Errorf("expected 1 APM Pod, got %d", len(pods))
					}
					previousPodUID = &pods[0].UID

					var apm apmv1.ApmServer
					if err := k.Client.Get(context.Background(), apmNamespacedName, &apm); err != nil {
						return err
					}
					customConfig := commonv1.Config{
						Data: map[string]interface{}{"output.elasticsearch.compression_level": 1},
					}
					apm.Spec.Config = &customConfig
					return k.Client.Update(context.Background(), &apm)
				}),
			},
			test.Step{
				Name: "APM Pod should be recreated",
				Test: test.Eventually(func() error {
					// get current pod id
					pods, err := k.GetPods(apmPodListOpts...)
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
				Test: test.Eventually(func() error {
					config, err := partialAPMConfiguration(k, namespace, apmName)
					if err != nil {
						return err
					}
					if config.Output.Elasticsearch.CompressionLevel != 1 { // value should be updated to 1
						return fmt.Errorf("expected compression level 1 but got %d", config.Output.Elasticsearch.CompressionLevel)
					}
					return nil
				}),
			},

			// cleanup extra resources
			test.Step{
				Name: "Delete secure settings secret",
				Test: test.Eventually(func() error {
					err := k.Client.Delete(context.Background(), &secureSettings)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
					return nil
				}),
			},
		}
	}

	test.Sequence(initStepsFn, stepsFn, esBuilder, apmBuilder).RunSequential(t)

}

func partialAPMConfiguration(k *test.K8sClient, namespace, name string) (PartialApmConfiguration, error) {
	var config PartialApmConfiguration
	// get current pods
	pods, err := k.GetPods(test.ApmServerPodListOptions(namespace, name)...)
	if err != nil {
		return config, err
	}
	if len(pods) == 0 {
		return config, errors.New("no pods found")
	}

	// exec into the pod to list keystore entries
	stdout, stderr, err := k.Exec(k8s.ExtractNamespacedName(&pods[0]),
		[]string{"cat", "/usr/share/apm-server/config/config-secret/apm-server.yml"})
	if err != nil {
		return config, errors.Wrap(err, fmt.Sprintf("stdout:\n%s\nstderr:\n%s", stdout, stderr))
	}
	err = yaml.Unmarshal([]byte(stdout), &config)
	if err != nil {
		return config, err
	}
	return config, nil
}
