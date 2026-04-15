// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	esClient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

type Monitored interface {
	Name() string
	Namespace() string
	GetMetricsIndexPattern() string
	GetLogsCluster() *types.NamespacedName
	GetMetricsCluster() *types.NamespacedName
}

func MonitoredSteps(monitored Monitored, k8sClient *test.K8sClient) test.StepList {
	return stackMonitoringChecks{
		monitored: monitored,
		k8sClient: k8sClient,
	}.Steps()
}

func BeatsMonitoredStep(monitored Monitored, k8sClient *test.K8sClient) test.Step {
	return stackMonitoringChecks{
		monitored: monitored,
		k8sClient: k8sClient,
	}.CheckMonitoringMetricsIndex()
}

// stackMonitoringChecks tests that the monitored resource pods have 3 containers ready and that there are documents indexed in the beat indexes
// of the monitoring Elasticsearch clusters.
type stackMonitoringChecks struct {
	monitored Monitored
	k8sClient *test.K8sClient
}

func (c stackMonitoringChecks) Steps() test.StepList {
	return test.StepList{
		c.CheckBeatSidecarsInElasticsearch(),
		c.CheckMonitoringMetricsIndex(),
		c.CheckFilebeatIndex(),
	}
}

func (c stackMonitoringChecks) CheckBeatSidecarsInElasticsearch() test.Step {
	return test.Step{
		Name: "Check that beat sidecars are running",
		Test: test.Eventually(func() error {
			pods, err := c.k8sClient.GetPods(
				test.ESPodListOptions(
					c.monitored.Namespace(),
					c.monitored.Name())...,
			)
			if err != nil {
				return err
			}
			for _, pod := range pods {
				if len(pod.Spec.Containers) != 3 {
					return fmt.Errorf("expected %d containers, got %d", 3, len(pod.Spec.Containers))
				}
				if !k8s.IsPodReady(pod) {
					return fmt.Errorf("pod %s not ready", pod.Name)
				}
			}
			return nil
		})}
}

func (c stackMonitoringChecks) CheckMonitoringMetricsIndex() test.Step {
	indexPattern := c.monitored.GetMetricsIndexPattern()
	return test.Step{
		Name: fmt.Sprintf("Check that documents are indexed in index %s", indexPattern),
		Test: test.Eventually(func() error {
			if c.monitored.GetMetricsCluster() == nil {
				return nil
			}
			esMetricsRef := *c.monitored.GetMetricsCluster()
			// Get Elasticsearch
			esMetrics := esv1.Elasticsearch{}
			if err := c.k8sClient.Client.Get(context.Background(), esMetricsRef, &esMetrics); err != nil {
				return err
			}
			// Create a new Elasticsearch client
			client, err := elasticsearch.NewElasticsearchClient(esMetrics, c.k8sClient)
			if err != nil {
				return err
			}
			// Check that there is at least one document
			err = containsDocuments(client, indexPattern)
			if err != nil {
				return err
			}
			return nil
		})}
}

func (c stackMonitoringChecks) CheckFilebeatIndex() test.Step {
	return test.Step{
		Name: "Check that documents are indexed in one filebeat-* index",
		Test: test.Eventually(func() error {
			if c.monitored.GetMetricsCluster() == nil {
				return nil
			}
			esLogsRef := *c.monitored.GetLogsCluster()
			// Get Elasticsearch
			esLogs := esv1.Elasticsearch{}
			if err := c.k8sClient.Client.Get(context.Background(), esLogsRef, &esLogs); err != nil {
				return err
			}
			// Create a new Elasticsearch client
			client, err := elasticsearch.NewElasticsearchClient(esLogs, c.k8sClient)
			if err != nil {
				return err
			}
			err = containsDocuments(client, "filebeat-*")
			if err != nil {
				return err
			}
			return nil
		})}
}

// QuerylogSteps returns steps that enable query logging on the monitored ES cluster, generate
// queries, verify that querylog events are indexed in the logs monitoring cluster, and then
// disable query logging again. Returns nil on versions before 9.4 where querylog is not available.
func QuerylogSteps(monitored, logs *elasticsearch.Builder, k *test.K8sClient) test.StepList {
	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if !v.GTE(version.MinFor(9, 4, 0)) {
		return nil
	}
	qc := querylogChecks{monitored: monitored, logs: logs, k8sClient: k}
	return test.StepList{
		qc.enableAndGenerateQueries(),
		qc.checkQuerylogIndex(),
		qc.disableQueryLog(),
	}
}

type querylogChecks struct {
	monitored *elasticsearch.Builder
	logs      *elasticsearch.Builder
	k8sClient *test.K8sClient
}

func (qc querylogChecks) esClient(es esv1.Elasticsearch) (esClient.Client, error) {
	return elasticsearch.NewElasticsearchClient(es, qc.k8sClient)
}

func (qc querylogChecks) setQueryLog(enabled bool) error {
	monitoredES := esv1.Elasticsearch{}
	ref := types.NamespacedName{Name: qc.monitored.Elasticsearch.Name, Namespace: qc.monitored.Elasticsearch.Namespace}
	if err := qc.k8sClient.Client.Get(context.Background(), ref, &monitoredES); err != nil {
		return err
	}
	client, err := qc.esClient(monitoredES)
	if err != nil {
		return err
	}
	body := strings.NewReader(fmt.Sprintf(`{"persistent":{"elasticsearch.querylog.enabled":%v}}`, enabled))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, "/_cluster/settings", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Request(context.Background(), req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (qc querylogChecks) generateQueries() error {
	monitoredES := esv1.Elasticsearch{}
	ref := types.NamespacedName{Name: qc.monitored.Elasticsearch.Name, Namespace: qc.monitored.Elasticsearch.Namespace}
	if err := qc.k8sClient.Client.Get(context.Background(), ref, &monitoredES); err != nil {
		return err
	}
	client, err := qc.esClient(monitoredES)
	if err != nil {
		return err
	}
	// Index a document to ensure there is data to query — searches against an empty cluster
	// (0 shards) do not generate querylog entries.
	indexReq, err := http.NewRequestWithContext(context.Background(), http.MethodPut, "/querylog-test/_doc/1?refresh=true", strings.NewReader(`{"msg":"querylog-test"}`))
	if err != nil {
		return err
	}
	indexReq.Header.Set("Content-Type", "application/json")
	indexResp, err := client.Request(context.Background(), indexReq)
	if err != nil {
		return err
	}
	indexResp.Body.Close()
	// Run searches to generate querylog entries
	for range 5 {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/querylog-test/_search", strings.NewReader(`{"query":{"match_all":{}}}`))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Request(context.Background(), req)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func (qc querylogChecks) enableAndGenerateQueries() test.Step {
	return test.Step{
		Name: "Enable query logging and generate queries",
		Test: test.Eventually(func() error {
			if err := qc.setQueryLog(true); err != nil {
				return err
			}
			return qc.generateQueries()
		}),
	}
}

func (qc querylogChecks) checkQuerylogIndex() test.Step {
	return test.Step{
		Name: "Check that querylog documents are indexed in logs-elasticsearch.querylog-default",
		Test: test.Eventually(func() error {
			// Keep generating queries on each retry to ensure Filebeat has data to ship
			if err := qc.generateQueries(); err != nil {
				return err
			}
			esLogs := esv1.Elasticsearch{}
			ref := types.NamespacedName{Name: qc.logs.Elasticsearch.Name, Namespace: qc.logs.Elasticsearch.Namespace}
			if err := qc.k8sClient.Client.Get(context.Background(), ref, &esLogs); err != nil {
				return err
			}
			client, err := qc.esClient(esLogs)
			if err != nil {
				return err
			}
			return hasDocumentsInDataStream(client, "logs-elasticsearch.querylog-default")
		}),
	}
}

func (qc querylogChecks) disableQueryLog() test.Step {
	return test.Step{
		Name: "Disable query logging",
		Test: test.Eventually(func() error {
			return qc.setQueryLog(false)
		}),
	}
}

// Index partially models Elasticsearch cluster index returned by /_cat/indices
type Index struct {
	Index     string `json:"index"`
	DocsCount string `json:"docs.count"`
}

// hasDocumentsInDataStream checks that a data stream exists and contains at least one document.
// Uses the _data_stream API to verify existence and _search to check for documents, following
// the same pattern as the agent e2e checks (see test/e2e/test/agent/checks.go).
func hasDocumentsInDataStream(client esClient.Client, dataStream string) error {
	// Check that the data stream exists
	dsReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("/_data_stream/%s", dataStream), nil)
	if err != nil {
		return err
	}
	dsResp, err := client.Request(context.Background(), dsReq)
	if err != nil {
		return err
	}
	defer dsResp.Body.Close()
	var dsResult struct {
		DataStreams []struct {
			Name string `json:"name"`
		} `json:"data_streams"`
	}
	if err := json.NewDecoder(dsResp.Body).Decode(&dsResult); err != nil {
		return err
	}
	if len(dsResult.DataStreams) == 0 {
		return fmt.Errorf("data stream [%s] does not exist", dataStream)
	}
	// Check that there is at least one document
	searchReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("/%s/_search?size=0", dataStream), nil)
	if err != nil {
		return err
	}
	searchResp, err := client.Request(context.Background(), searchReq)
	if err != nil {
		return err
	}
	defer searchResp.Body.Close()
	var searchResult struct {
		Hits struct {
			Total struct {
				Value int `json:"value"`
			} `json:"total"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(searchResp.Body).Decode(&searchResult); err != nil {
		return err
	}
	if searchResult.Hits.Total.Value == 0 {
		return fmt.Errorf("data stream [%s] has no documents", dataStream)
	}
	return nil
}

func containsDocuments(esClient esClient.Client, indexPattern string) error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/_cat/indices/%s?format=json", indexPattern), nil) //nolint:noctx
	if err != nil {
		return err
	}
	resp, err := esClient.Request(context.Background(), req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	resultBytes, err := io.ReadAll(resp.Body)
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
		return fmt.Errorf("expected [%d] index [%s], found [%d]", 1, indexPattern, len(indices))
	}
	docsCount, err := strconv.Atoi(indices[0].DocsCount)
	if err != nil {
		return err
	}
	// with at least 1 doc
	if docsCount <= 0 {
		return fmt.Errorf("index [%s] empty", indexPattern)
	}
	return nil
}
