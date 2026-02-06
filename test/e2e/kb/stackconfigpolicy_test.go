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

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/kibana"
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

// TestStackConfigPolicyKibanaMultipleWeights tests multiple StackConfigPolicies with different weights for Kibana.
func TestStackConfigPolicyKibanaMultipleWeights(t *testing.T) {
	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	namespace := test.Ctx().ManagedNamespace(0)
	// set up a 1-node Kibana deployment
	name := "test-kb-scp-multi"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1).WithLabel("app", "kibana")

	kbPodListOpts := test.KibanaPodListOptions(kbBuilder.Kibana.Namespace, kbBuilder.Kibana.Name)

	// Policy with weight 10 (lower priority)
	lowPriorityPolicy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("low-priority-kb-scp-%s", rand.String(4)),
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			Weight: 10,
			ResourceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "kibana"},
			},
			Kibana: policyv1alpha1.KibanaConfigPolicySpec{
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"server.customResponseHeaders": map[string]interface{}{
							"priority":    "low",
							"test-header": "low-priority-value",
						},
					},
				},
				SecureSettings: []commonv1.SecretSource{
					{SecretName: fmt.Sprintf("low-priority-secret-%s", rand.String(4))},
				},
			},
		},
	}

	// Policy with weight 20 (higher priority) - should override lower priority settings
	highPriorityPolicy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("high-priority-kb-scp-%s", rand.String(4)),
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			Weight: 20,
			ResourceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "kibana"},
			},
			Kibana: policyv1alpha1.KibanaConfigPolicySpec{
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"server.customResponseHeaders": map[string]interface{}{
							"priority":    "high",
							"test-header": "high-priority-value",
						},
					},
				},
				SecureSettings: []commonv1.SecretSource{
					{SecretName: fmt.Sprintf("high-priority-secret-%s", rand.String(4))},
				},
			},
		},
	}

	// Policy with same weight 10 but different selector (should not conflict)
	nonConflictingPolicy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("non-conflicting-kb-scp-%s", rand.String(4)),
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			Weight: 10,
			ResourceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "elasticsearch"}, // Different selector
			},
			Kibana: policyv1alpha1.KibanaConfigPolicySpec{
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"server.customResponseHeaders": map[string]interface{}{
							"priority": "should-not-apply",
						},
					},
				},
			},
		},
	}

	// Create secure settings secrets
	lowPrioritySecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lowPriorityPolicy.Spec.Kibana.SecureSettings[0].SecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"elasticsearch.pingTimeout": []byte("30000"),
		},
	}

	highPrioritySecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      highPriorityPolicy.Spec.Kibana.SecureSettings[0].SecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"elasticsearch.requestTimeout": []byte("30000"),
		},
	}

	esWithlicense := test.LicenseTestBuilder(esBuilder)

	steps := func(k *test.K8sClient) test.StepList {
		kibanaChecks := kibana.KbChecks{
			Client: k,
		}
		return test.StepList{
			test.Step{
				Name: "Create secure settings secrets",
				Test: test.Eventually(func() error {
					if err := k.CreateOrUpdate(&lowPrioritySecret); err != nil {
						return err
					}
					return k.CreateOrUpdate(&highPrioritySecret)
				}),
			},
			test.Step{
				Name: "Create low priority StackConfigPolicy",
				Test: test.Eventually(func() error {
					return k.CreateOrUpdate(&lowPriorityPolicy)
				}),
			},
			test.Step{
				Name: "Create high priority StackConfigPolicy",
				Test: test.Eventually(func() error {
					return k.CreateOrUpdate(&highPriorityPolicy)
				}),
			},
			test.Step{
				Name: "Create non-conflicting StackConfigPolicy",
				Test: test.Eventually(func() error {
					return k.CreateOrUpdate(&nonConflictingPolicy)
				}),
			},
			// High priority settings should be applied
			kibanaChecks.CheckHeaderForKey(kbBuilder, "priority", "high"),
			kibanaChecks.CheckHeaderForKey(kbBuilder, "test-header", "high-priority-value"),
			// High priority secure settings should be in keystore
			test.CheckKeystoreEntries(k, KibanaKeystoreCmd, []string{"elasticsearch.pingTimeout", "elasticsearch.requestTimeout"}, kbPodListOpts...),
			test.Step{
				Name: "Delete high priority policy - low priority should take effect",
				Test: test.Eventually(func() error {
					return k.Client.Delete(context.Background(), &highPriorityPolicy)
				}),
			},
			// Low priority settings should now be applied
			kibanaChecks.CheckHeaderForKey(kbBuilder, "priority", "low"),
			kibanaChecks.CheckHeaderForKey(kbBuilder, "test-header", "low-priority-value"),
			// Low priority secure settings should be in keystore
			test.CheckKeystoreEntries(k, KibanaKeystoreCmd, []string{"elasticsearch.pingTimeout"}, kbPodListOpts...),
			test.Step{
				Name: "Clean up remaining policies and secrets",
				Test: test.Eventually(func() error {
					if err := k.Client.Delete(context.Background(), &lowPriorityPolicy); err != nil {
						return err
					}
					if err := k.Client.Delete(context.Background(), &nonConflictingPolicy); err != nil {
						return err
					}
					if err := k.Client.Delete(context.Background(), &lowPrioritySecret); err != nil {
						return err
					}
					return k.Client.Delete(context.Background(), &highPrioritySecret)
				}),
			},
		}
	}

	test.Sequence(nil, steps, esWithlicense, kbBuilder).RunSequential(t)
}
