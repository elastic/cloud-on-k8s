// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build logstash || e2e

package logstash

import (
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"testing"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/checks"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/logstash"
)

// TestLogstashStackMonitoring tests that when Logstash is configured with monitoring, its log and metrics are
// correctly delivered to the referenced monitoring Elasticsearch clusters.
func TestLogstashStackMonitoring(t *testing.T) {
	// only execute this test on supported version
	if version.MustParse(test.Ctx().ElasticStackVersion).LT(logstashv1alpha1.MinStackMonVersion) {
		t.SkipNow()
	}

	// create 1 monitored and 2 monitoring clusters to collect separately metrics and logs
	metrics := elasticsearch.NewBuilder("test-ls-mon-metrics").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)
	logs := elasticsearch.NewBuilder("test-ls-mon-logs").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)
	monitored := logstash.NewBuilder("test-ls-mon-a").
		WithNodeCount(1).
		WithMetricsMonitoring(metrics.Ref()).
		WithLogsMonitoring(logs.Ref())

	// checks that the sidecar beats have sent data in the monitoring clusters
	steps := func(k *test.K8sClient) test.StepList {
		return checks.MonitoredSteps(&monitored, k)
	}

	test.Sequence(nil, steps, metrics, logs, monitored).RunSequential(t)
}
