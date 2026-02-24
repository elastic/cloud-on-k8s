// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build autoops || e2e

package autoops

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/autoops"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

func TestAutoOpsAgentPolicy(t *testing.T) {
	runAutoOpsAgentPolicyTest(t, false)
}

func TestAutoOpsAgentPolicyEnterprise(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping enterprise AutoOps test: no enterprise test license configured")
	}
	runAutoOpsAgentPolicyTest(t, true)
}

func runAutoOpsAgentPolicyTest(t *testing.T, useEnterpriseLicense bool) {
	t.Helper()

	minSupportedVersion := version.SupportedAutoOpsAgentBasicVersions.Min
	if useEnterpriseLicense {
		minSupportedVersion = version.SupportedAutoOpsAgentEnterpriseVersions.Min
	}

	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if v.LT(minSupportedVersion) {
		t.Skipf("Skipping test: Elastic Stack version %s is below minimum supported version %s",
			test.Ctx().ElasticStackVersion, minSupportedVersion)
	}

	// Use separate namespaces for ES and policy
	esNamespace := test.Ctx().ManagedNamespace(0)
	policyNamespace := test.Ctx().ManagedNamespace(1)

	esName := "es"
	es1Builder := elasticsearch.NewBuilderWithoutSuffix(esName).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithNamespace(esNamespace).
		WithVersion(test.Ctx().ElasticStackVersion).
		WithLabel("autoops", "enabled")

	// 2nd elasticsearch cluster that should be omitted from autoops based on namespace
	es2Builder := elasticsearch.NewBuilder("ex-es").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithNamespace(policyNamespace).
		WithVersion(test.Ctx().ElasticStackVersion).
		WithLabel("autoops", "enabled")

	// Create the policy builder with the mock URL for cloud-connected API and OTel
	mockURL := autoops.CloudConnectedAPIMockURL()
	policyBuilder := autoops.NewBuilder("autoops-policy").
		WithNamespace(policyNamespace).
		WithResourceSelector(metav1.LabelSelector{
			MatchLabels: map[string]string{
				"autoops": "enabled",
			},
		}).WithNamespaceSelector(metav1.LabelSelector{
		MatchLabels: map[string]string{
			"kubernetes.io/metadata.name": esNamespace,
		},
	}).WithCloudConnectedAPIURL(mockURL).
		WithAutoOpsOTelURL(mockURL)

	var policyTestBuilder test.Builder = policyBuilder
	if useEnterpriseLicense {
		policyTestBuilder = test.LicenseTestBuilder(policyBuilder)
	}

	before := test.EmptySteps
	if !useEnterpriseLicense {
		before = func(k *test.K8sClient) test.StepList {
			return test.StepList{
				{
					Name: "Remove enterprise license secrets for non-enterprise scenario",
					Test: func(t *testing.T) {
						test.DeleteAllEnterpriseLicenseSecrets(t, k)
					},
				},
			}
		}
	}

	test.Sequence(before, test.EmptySteps, es1Builder, es2Builder, policyTestBuilder).
		RunSequential(t)
}
