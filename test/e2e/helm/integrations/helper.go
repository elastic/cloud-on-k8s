// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build chart_integrations || e2e

package integrations

import (
	"strings"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
)

func getElasticSearchBuilder(namespace string, suffix string, fullTestName string, version string) (elasticsearch.Builder, error) {
	esBuilder := elasticsearch.NewBuilderWithoutSuffix("integrations-es")
	esBuilder.Elasticsearch.Spec = esv1.ElasticsearchSpec{
		Version: version,
		NodeSets: []esv1.NodeSet{
			{
				Name:  "default",
				Count: 3,
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"node.store.allow_mmap": false,
					},
				},
			},
		},
	}
	esBuilder = esBuilder.WithNamespace(namespace).
		WithSuffix(suffix).
		WithRestrictedSecurityContext().
		WithLabel(run.TestNameLabel, fullTestName).
		WithPodLabel(run.TestNameLabel, fullTestName)

	if strings.HasPrefix(test.Ctx().Provider, "eks") {
		esBuilder = esBuilder.WithDefaultPersistentVolumes()
	}

	return esBuilder, nil
}
