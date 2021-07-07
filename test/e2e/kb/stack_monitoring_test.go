// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/stackmon/validations"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/checks"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
)

// TestKBStackMonitoring tests that when a Kibana is configured with monitoring, its log and metrics are
// correctly delivered to the referenced monitoring Elasticsearch clusters.
func TestKBStackMonitoring(t *testing.T) {
	// only execute this test on supported version
	err := validations.IsSupportedVersion(test.Ctx().ElasticStackVersion)
	if err != nil {
		t.SkipNow()
	}

	// create 1 monitored and 2 monitoring clusters to collect separately metrics and logs
	metrics := elasticsearch.NewBuilder("test-es-mon-metrics").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)
	logs := elasticsearch.NewBuilder("test-es-mon-logs").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)
	assocEs := elasticsearch.NewBuilder("test-es-mon-a").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	monitored := kibana.NewBuilder("test-es-mon-a").
		WithElasticsearchRef(assocEs.Ref()).
		WithNodeCount(1).
		WithMonitoring(metrics.Ref(), logs.Ref())

	// checks that the sidecar beats have sent data in the monitoring clusters
	steps := func(k *test.K8sClient) test.StepList {
		return checks.StackMonitoringChecks{
			MonitoredNsn: k8s.ExtractNamespacedName(&assocEs.Elasticsearch),
			Metrics:      metrics, Logs: logs, K: k,
		}.Steps()
	}

	test.Sequence(nil, steps, metrics, logs, monitored).RunSequential(t)
}
