// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	esClient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"k8s.io/apimachinery/pkg/types"
)

// StackMonitoringChecks tests that the monitored resource pods have 3 containers ready and that there are documents indexed in the beat indexes
// of the monitoring Elasticsearch clusters.
type StackMonitoringChecks struct {
	MonitoredNsn types.NamespacedName
	Metrics      elasticsearch.Builder
	Logs         elasticsearch.Builder
	K            *test.K8sClient
}

func (c StackMonitoringChecks) Steps() test.StepList {
	return test.StepList{
		c.CheckBeatSidecars(),
		c.CheckMetricbeatIndex(),
		c.CheckFilebeatIndex(),
	}
}

func (c StackMonitoringChecks) CheckBeatSidecars() test.Step {
	return test.Step{
		Name: "Check that beat sidecars are running",
		Test: test.Eventually(func() error {
			pods, err := c.K.GetPods(test.ESPodListOptions(c.MonitoredNsn.Namespace, c.MonitoredNsn.Name)...)
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

func (c StackMonitoringChecks) CheckMetricbeatIndex() test.Step {
	return test.Step{
		Name: "Check that documents are indexed in one metricbeat-* index",
		Test: test.Eventually(func() error {
			client, err := elasticsearch.NewElasticsearchClient(c.Metrics.Elasticsearch, c.K)
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

func (c StackMonitoringChecks) CheckFilebeatIndex() test.Step {
	return test.Step{
		Name: "Check that documents are indexed in one filebeat-* index",
		Test: test.Eventually(func() error {
			client, err := elasticsearch.NewElasticsearchClient(c.Logs.Elasticsearch, c.K)
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
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/_cat/indices/%s?format=json", indexPattern), nil) //nolint:noctx
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
	if docsCount <= 0 {
		return fmt.Errorf("index [%s] empty", indexPattern)
	}

	return nil
}
