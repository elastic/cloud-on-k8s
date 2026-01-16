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
	// https://github.com/elastic/cloud-on-k8s/issues/9027
	// t.Skip("Skipping AutoOpsAgentPolicy test")

	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	// only execute this test with supported AutoOps versions
	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if v.LT(version.SupportedAutoOpsAgentVersions.Min) {
		t.SkipNow()
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

	es1Withlicense := test.LicenseTestBuilder(es1Builder)

	// 2nd elasticsearch cluster that should be omitted from autoops based on namespace
	es2Builder := elasticsearch.NewBuilder("ex-es").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithNamespace(policyNamespace).
		WithVersion(test.Ctx().ElasticStackVersion).
		WithLabel("autoops", "enabled")

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
	})

	test.Sequence(nil, test.EmptySteps, es1Withlicense, es2Builder, policyBuilder).
		RunSequential(t)
}
