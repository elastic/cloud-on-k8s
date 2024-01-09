// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build kb || e2e

package kb

import (
	"context"
	_ "embed"
	"fmt"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/kibana"
)

var (
	//go:embed fixtures/stackconfigpolicy_kb.yaml
	kbConfig string
)

// TestStackConfigPolicy tests the StackConfigPolicy feature for Kibana.
func TestStackConfigPolicyKibana(t *testing.T) {
	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	namespace := test.Ctx().ManagedNamespace(0)
	// set up a 1-node Kibana deployment
	name := "test-kb-scp"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1).WithLabel("label", "test-scp")

	kbPodListOpts := test.KibanaPodListOptions(kbBuilder.Kibana.Namespace, kbBuilder.Kibana.Name)
	secureSettingsSecretName := fmt.Sprintf("test-scp-secure-settings-%s", rand.String(4))

	// set the policy Kibana settings the policy using the external YAML file
	var kibanaConfigSpec policyv1alpha1.KibanaConfigPolicySpec
	err := yaml.Unmarshal([]byte(kbConfig), &kibanaConfigSpec)
	assert.NoError(t, err)

	kibanaConfigSpec.SecureSettings = []commonv1.SecretSource{
		{SecretName: secureSettingsSecretName},
	}

	policy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("test-scp-%s", rand.String(4)),
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			ResourceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"label": "test-scp"},
			},
			Kibana: kibanaConfigSpec,
		},
	}

	secureSettingsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secureSettingsSecretName,
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Data: map[string][]byte{
			// this needs to be a valid configuration item, otherwise Kibana refuses to start
			"elasticsearch.pingTimeout": []byte("30000"),
		},
	}

	esWithlicense := test.LicenseTestBuilder(esBuilder)

	steps := func(k *test.K8sClient) test.StepList {
		kibanaChecks := kibana.KbChecks{
			Client: k,
		}
		return test.StepList{
			test.Step{
				Name: "Create a Secure Settings secret",
				Test: test.Eventually(func() error {
					err := k.CreateOrUpdate(&secureSettingsSecret)
					return err
				}),
			},
			test.Step{
				Name: "Create a StackConfigPolicy",
				Test: test.Eventually(func() error {
					err := k.CreateOrUpdate(&policy)
					return err
				}),
			},
			test.CheckKeystoreEntries(k, KibanaKeystoreCmd, []string{"elasticsearch.pingTimeout"}, kbPodListOpts...),
			// We set test: kb-scp-test as a config for server.customResponseHeaders, so we should that see that in the response headers from Kibana
			kibanaChecks.CheckHeaderForKey(kbBuilder, "test", "kb-scp-test"),
			test.Step{
				Name: "Deleting the StackConfigPolicy should return no error",
				Test: test.Eventually(func() error {
					return k.Client.Delete(context.Background(), &policy)
				}),
			},
			// keystore should be reset
			test.CheckKeystoreEntries(k, KibanaKeystoreCmd, nil, kbPodListOpts...),
			test.Step{
				Name: "Delete secure settings secret",
				Test: test.Eventually(func() error {
					return k.Client.Delete(context.Background(), &secureSettingsSecret)
				}),
			},
		}
	}

	test.Sequence(nil, steps, esWithlicense, kbBuilder).RunSequential(t)
}
