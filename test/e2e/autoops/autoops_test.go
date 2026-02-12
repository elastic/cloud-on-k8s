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
	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping test: no test license provided")
	}

	// only execute this test with supported AutoOps versions
	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if v.LT(version.SupportedAutoOpsAgentVersions.Min) {
		t.Skipf("Skipping test: Elastic Stack version %s is below minimum supported version %s",
			test.Ctx().ElasticStackVersion, version.SupportedAutoOpsAgentVersions.Min)
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

	test.Sequence(nil, test.EmptySteps, es1Builder, es2Builder, policyBuilder).
		RunSequential(t)
}
