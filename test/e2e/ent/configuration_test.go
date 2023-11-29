// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build ent || e2e

package ent

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/enterprisesearch"
)

// TestEnterpriseSearchConfigUpdate updates an existing EnterpriseSearch deployment twice:
// 1. with an additional config
// 2. with an additional configRef
func TestEnterpriseSearchConfigUpdate(t *testing.T) {
	name := "test-ent-config-ref"
	es := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithRestrictedSecurityContext()

	// initial Enterprise Search with no custom config
	entNoConfig := enterprisesearch.NewBuilder(name).
		WithElasticsearchRef(es.Ref()).
		WithNodeCount(1).
		WithRestrictedSecurityContext()

	// 1. additional config setting
	entWithConfig := enterprisesearch.Builder{EnterpriseSearch: *entNoConfig.EnterpriseSearch.DeepCopy()}.
		WithConfig(map[string]interface{}{"app_search.engine.document_size.limit": "100kb"}).
		WithMutatedFrom(&entNoConfig)
	var expectedAdditionalConfig PartialConfig
	err := yaml.Unmarshal([]byte(`app_search:
  engine:
    document_size:
      limit: 100kb`), &expectedAdditionalConfig)
	require.NoError(t, err)

	// 2. additional configRef
	configRefSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ent-smtp-credentials",
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Data: map[string][]byte{
			"enterprise-search.yml": []byte(`email.account.enabled: true
email.account.smtp.auth: plain
email.account.smtp.starttls.enable: false
email.account.smtp.host: 127.0.0.1
email.account.smtp.port: 25
email.account.smtp.user: myuser
email.account.smtp.password: mypassword
email.account.email_defaults.from: my@email.com`),
		},
	}
	var expectedAdditionalCfgRef PartialConfig
	// contains custom config + custom configRef (from which we just check one entry)
	err = yaml.Unmarshal([]byte(
		`app_search:
  engine:
    document_size:
      limit: 100kb
email:
  account:
    smtp:
      password: mypassword`),
		&expectedAdditionalCfgRef,
	)
	require.NoError(t, err)
	entWithConfigRef := enterprisesearch.Builder{EnterpriseSearch: *entWithConfig.EnterpriseSearch.DeepCopy()}.
		WithConfigRef(&commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: configRefSecret.Name}}).
		WithMutatedFrom(&entWithConfig)

	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{}.
			// mutate with additional config
			WithSteps(entWithConfig.MutationTestSteps(k)).
			WithStep(test.Step{
				Name: "Config file in the Pod should contain the additional config",
				Test: test.Eventually(func() error {
					return CheckPartialConfig(k, entWithConfig.EnterpriseSearch, expectedAdditionalConfig)
				}),
			}).
			// mutate with additional configRef
			WithStep(test.Step{
				Name: "Create configRef secret",
				Test: test.Eventually(func() error {
					return k.CreateOrUpdate(&configRefSecret)
				}),
			}).
			WithSteps(entWithConfigRef.MutationTestSteps(k)).
			WithStep(test.Step{
				Name: "Config file in the Pod should contain the additional config & configRef",
				Test: test.Eventually(func() error {
					return CheckPartialConfig(k, entWithConfigRef.EnterpriseSearch, expectedAdditionalCfgRef)
				}),
			})
	}

	test.Sequence(nil, stepsFn, es, entNoConfig).RunSequential(t)
}

// CheckPartialConfig retrieves the configuration file from all Pods and compares it with the expected PartialConfig.
func CheckPartialConfig(k *test.K8sClient, ent entv1.EnterpriseSearch, expected PartialConfig) error {
	var pods corev1.PodList
	err := k.Client.List(context.Background(), &pods, test.EnterpriseSearchPodListOptions(ent.Namespace, ent.Name)...)
	if err != nil {
		return err
	}
	if len(pods.Items) == 0 {
		return errors.New("found 0 pod to check config from")
	}
	for _, p := range pods.Items {
		cfg, err := GetConfigFromPod(k, types.NamespacedName{Namespace: p.Namespace, Name: p.Name})
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(expected, cfg) {
			return fmt.Errorf("expected config %+v, got %+v", expected, cfg)
		}
	}
	return nil
}

// PartialConfig partially models an Enterprise Search configuration for test needs.
type PartialConfig struct {
	AppSearch struct {
		Engine struct {
			DocumentSize struct {
				Limit string `yaml:"limit"`
			} `yaml:"document_size"`
		} `yaml:"engine"`
	} `yaml:"app_search"`
	Email struct {
		Account struct {
			SMTP struct {
				Password string `yaml:"password"`
			} `yaml:"smtp"`
		} `yaml:"account"`
	} `yaml:"email"`
}

// GetConfigFromPod execs into the Pod to retrieve the Enterprise Search configuration file.
func GetConfigFromPod(k *test.K8sClient, pod types.NamespacedName) (PartialConfig, error) {
	stdout, stderr, err := k.Exec(pod, []string{"cat", "/usr/share/enterprise-search/config/enterprise-search.yml"})
	if err != nil {
		return PartialConfig{}, errors.Wrap(err, fmt.Sprintf("failed to get config from Pod, stdout: %s; stderr: %s", stdout, stderr))
	}
	var cfg PartialConfig
	return cfg, yaml.Unmarshal([]byte(stdout), &cfg)
}
