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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	esClient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
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
	steps := test.StepList{
		c.CheckBeatSidecarsInElasticsearch(),
		c.CheckMonitoringMetricsIndex(),
		c.CheckFilebeatIndex(),
	}
	// On 9.4+ verify that querylog events can be shipped to the monitoring cluster
	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if v.GTE(version.MinFor(9, 4, 0)) {
		steps = append(steps, c.EnableQueryLogAndGenerateQueries(), c.CheckQuerylogIndex(), c.DisableQueryLog())
	}
	return steps
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
			err = ContainsDocuments(client, indexPattern)
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
			err = ContainsDocuments(client, "filebeat-*")
			if err != nil {
				return err
			}
			return nil
		})}
}

// monitoredESClient returns an ES client for the monitored resource if it is an Elasticsearch cluster.
// Returns nil, nil if the monitored resource is not an Elasticsearch cluster.
func (c stackMonitoringChecks) monitoredESClient() (esClient.Client, error) {
	monitoredES := esv1.Elasticsearch{}
	ref := types.NamespacedName{Name: c.monitored.Name(), Namespace: c.monitored.Namespace()}
	if err := c.k8sClient.Client.Get(context.Background(), ref, &monitoredES); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return elasticsearch.NewElasticsearchClient(monitoredES, c.k8sClient)
}

func (c stackMonitoringChecks) setQueryLog(enabled bool) error {
	client, err := c.monitoredESClient()
	if err != nil {
		return err
	}
	if client == nil {
		return nil
	}
	body := strings.NewReader(fmt.Sprintf(`{"persistent":{"elasticsearch.querylog.enabled":%v}}`, enabled))
	req, err := http.NewRequest(http.MethodPut, "/_cluster/settings", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	_, err = client.Request(context.Background(), req)
	return err
}

func (c stackMonitoringChecks) EnableQueryLogAndGenerateQueries() test.Step {
	return test.Step{
		Name: "Enable query logging and generate queries",
		Test: test.Eventually(func() error {
			if err := c.setQueryLog(true); err != nil {
				return err
			}
			client, err := c.monitoredESClient()
			if err != nil {
				return err
			}
			if client == nil {
				return nil
			}
			for range 5 {
				req, err := http.NewRequest(http.MethodGet, "/_search", strings.NewReader(`{"query":{"match_all":{}}}`))
				if err != nil {
					return err
				}
				req.Header.Set("Content-Type", "application/json")
				if _, err = client.Request(context.Background(), req); err != nil {
					return err
				}
			}
			return nil
		}),
	}
}

func (c stackMonitoringChecks) CheckQuerylogIndex() test.Step {
	return test.Step{
		Name: "Check that querylog documents are indexed in logs-elasticsearch.querylog-*",
		Test: test.Eventually(func() error {
			if c.monitored.GetLogsCluster() == nil {
				return nil
			}
			esLogsRef := *c.monitored.GetLogsCluster()
			esLogs := esv1.Elasticsearch{}
			if err := c.k8sClient.Client.Get(context.Background(), esLogsRef, &esLogs); err != nil {
				return err
			}
			client, err := elasticsearch.NewElasticsearchClient(esLogs, c.k8sClient)
			if err != nil {
				return err
			}
			return ContainsDocuments(client, "logs-elasticsearch.querylog-*")
		}),
	}
}

func (c stackMonitoringChecks) DisableQueryLog() test.Step {
	return test.Step{
		Name: "Disable query logging",
		Test: test.Eventually(func() error {
			return c.setQueryLog(false)
		}),
	}
}

// Index partially models Elasticsearch cluster index returned by /_cat/indices
type Index struct {
	Index     string `json:"index"`
	DocsCount string `json:"docs.count"`
}

func ContainsDocuments(esClient esClient.Client, indexPattern string) error {
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
