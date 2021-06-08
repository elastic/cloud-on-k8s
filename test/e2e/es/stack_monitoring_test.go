// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"testing"

	esClient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/stackmon"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
)

// TestStackMonitoring
func TestStackMonitoring(t *testing.T) {
	// only execute this test on supported version
	err := stackmon.IsSupportedVersion(test.Ctx().ElasticStackVersion)
	if err != nil {
		t.SkipNow()
	}

	// Create 1 monitored and 2 monitoring clusters to collect separately metrics and logs
	metrics := elasticsearch.NewBuilder("test-es-mon-metrics").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)
	logs := elasticsearch.NewBuilder("test-es-mon-logs").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)
	monitored := elasticsearch.NewBuilder("test-es-mon-a").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithMonitoring(metrics.Ref(), logs.Ref())

	// Checks that the beats have sent data in the monitoring clusters
	steps := func(k *test.K8sClient) test.StepList {
		checks := stackMonitoringChecks{metrics, logs, k}
		return test.StepList{
			checks.CheckMetricbeatIndex(),
			checks.CheckFilebeatIndex(),
		}
	}

	test.Sequence(nil, steps, metrics, logs, monitored).RunSequential(t)
}

type stackMonitoringChecks struct {
	metrics elasticsearch.Builder
	logs    elasticsearch.Builder
	k       *test.K8sClient
}

func (c *stackMonitoringChecks) CheckMetricbeatIndex() test.Step {
	return test.Step{
		Name: "Check that documents are indexed in one metricbeat-* index",
		Test: test.Eventually(func() error {
			client, err := elasticsearch.NewElasticsearchClient(c.metrics.Elasticsearch, c.k)
			if err != nil {
				return err
			}
			err = AreIndexedDocs(client, "metricbeat-*")
			if err != nil {
				return err
			}
			return nil
		})}
}

func (c *stackMonitoringChecks) CheckFilebeatIndex() test.Step {
	return test.Step{
		Name: "Check that documents are indexed in one filebeat-* index",
		Test: test.Eventually(func() error {
			client, err := elasticsearch.NewElasticsearchClient(c.logs.Elasticsearch, c.k)
			if err != nil {
				return err
			}
			err = AreIndexedDocs(client, "filebeat*")
			if err != nil {
				return err
			}
			return nil
		})}
}

// Index partially models Elasticsearch cluster index returned by /_cat/indices
type Index struct {
	Index     string `json:"index"`
	DocsCount string `json:"docs.count"`
}

func AreIndexedDocs(esClient esClient.Client, indexPattern string) error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/_cat/indices/%s?health=green&format=json", indexPattern), nil) //nolint:noctx
	if err != nil {
		return err
	}
	resp, err := esClient.Request(context.Background(), req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	resultBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var indices []Index
	err = json.Unmarshal(resultBytes, &indices)
	if err != nil {
		return err
	}

	// 1 index must exist
	if len(indices) != 1 {
		return fmt.Errorf("expected [%d] index [%s], found [%d]", len(indices), indexPattern, 1)
	}
	docsCount, err := strconv.Atoi(indices[0].DocsCount)
	if err != nil {
		return err
	}
	// with at least 1 doc
	if docsCount < 0 {
		return fmt.Errorf("index [%s] empty", indexPattern)
	}

	return nil
}
