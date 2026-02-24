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
	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if v.LT(version.SupportedAutoOpsAgentBasicVersions.Min) {
		t.Skipf("Skipping test: version %s below minimum %s",
			test.Ctx().ElasticStackVersion, version.SupportedAutoOpsAgentBasicVersions.Min)
	}

	es1Builder, es2Builder, policyBuilder := autoOpsBuilders(t)

	before := func(k *test.K8sClient) test.StepList {
		return test.StepList{{
			Name: "Remove enterprise license secrets for non-enterprise scenario",
			Test: func(t *testing.T) { test.DeleteAllEnterpriseLicenseSecrets(t, k) },
		}}
	}

	test.Sequence(before, test.EmptySteps, es1Builder, es2Builder, policyBuilder).
		RunSequential(t)
}

func TestAutoOpsAgentPolicyEnterprise(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping enterprise AutoOps test: no enterprise test license configured")
	}

	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if v.LT(version.SupportedAutoOpsAgentEnterpriseVersions.Min) {
		t.Skipf("Skipping test: version %s below minimum %s",
			test.Ctx().ElasticStackVersion, version.SupportedAutoOpsAgentEnterpriseVersions.Min)
	}

	es1Builder, es2Builder, policyBuilder := autoOpsBuilders(t)

	test.Sequence(nil, test.EmptySteps, es1Builder, es2Builder, test.LicenseTestBuilder(policyBuilder)).
		RunSequential(t)
}

func autoOpsBuilders(t *testing.T) (elasticsearch.Builder, elasticsearch.Builder, autoops.Builder) {
	t.Helper()
	esNamespace := test.Ctx().ManagedNamespace(0)
	policyNamespace := test.Ctx().ManagedNamespace(1)
	mockURL := autoops.CloudConnectedAPIMockURL()

	es1 := elasticsearch.NewBuilderWithoutSuffix("es").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithNamespace(esNamespace).
		WithVersion(test.Ctx().ElasticStackVersion).
		WithLabel("autoops", "enabled")

	es2 := elasticsearch.NewBuilder("ex-es").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithNamespace(policyNamespace).
		WithVersion(test.Ctx().ElasticStackVersion).
		WithLabel("autoops", "enabled")

	policy := autoops.NewBuilder("autoops-policy").
		WithNamespace(policyNamespace).
		WithResourceSelector(metav1.LabelSelector{
			MatchLabels: map[string]string{"autoops": "enabled"},
		}).WithNamespaceSelector(metav1.LabelSelector{
		MatchLabels: map[string]string{"kubernetes.io/metadata.name": esNamespace},
	}).WithCloudConnectedAPIURL(mockURL).
		WithAutoOpsOTelURL(mockURL)

	return es1, es2, policy
}
