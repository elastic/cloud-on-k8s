// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	esClient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// CheckDeployment checks the Deployment resource exists
func CheckDeployment(subj Subject, k *K8sClient, deploymentName string) Step {
	return Step{
		Name: subj.Kind() + " deployment should be created",
		Test: Eventually(func() error {
			var dep appsv1.Deployment
			err := k.Client.Get(context.Background(), types.NamespacedName{
				Namespace: subj.NSN().Namespace,
				Name:      deploymentName,
			}, &dep)
			if subj.Count() == 0 && apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if *dep.Spec.Replicas != subj.Count() {
				return fmt.Errorf("invalid %s replicas count: expected %d, got %d", subj.Kind(), subj.Count(), *dep.Spec.Replicas)
			}
			return nil
		}),
	}
}

// CheckPods checks that the test subject's expected pods are eventually ready.
func CheckPods(subj Subject, k *K8sClient) Step {
	// This is a shared test but it is common for Enterprise Search Pods to take some time to be ready, especially
	// during the initial bootstrap, or during version upgrades. Let's increase the timeout
	// for this particular step.
	timeout := Ctx().TestTimeout * 2
	return Step{
		Name: subj.Kind() + " Pods should eventually be ready",
		Test: UntilSuccess(func() error {
			var pods corev1.PodList
			if err := k.Client.List(context.Background(), &pods, subj.ListOptions()...); err != nil {
				return err
			}

			// builder hash matches
			expectedBuilderHash := hash.HashObject(subj.Spec())
			for _, pod := range pods.Items {
				if err := ValidateBuilderHashAnnotation(pod, expectedBuilderHash); err != nil {
					return err
				}
			}

			// pod count matches
			if len(pods.Items) != int(subj.Count()) {
				return fmt.Errorf("invalid %s pod count: expected %d, got %d", subj.NSN().Name, subj.Count(), len(pods.Items))
			}

			// pods are running
			for _, pod := range pods.Items {
				if pod.Status.Phase != corev1.PodRunning {
					return fmt.Errorf("pod not running yet")
				}
			}

			// pods are ready
			for _, pod := range pods.Items {
				if !k8s.IsPodReady(pod) {
					return fmt.Errorf("pod not ready yet")
				}
			}

			return nil
		}, timeout),
	}
}

// CheckServices checks that all expected services have been created
func CheckServices(subj Subject, k *K8sClient) Step {
	return Step{
		Name: subj.Kind() + " services should be created",
		Test: Eventually(func() error {
			for _, s := range []string{
				subj.ServiceName(),
			} {
				if _, err := k.GetService(subj.NSN().Namespace, s); err != nil {
					return err
				}
			}
			return nil
		}),
	}
}

// CheckServicesEndpoints checks that services have the expected number of endpoints
func CheckServicesEndpoints(subj Subject, k *K8sClient) Step {
	return Step{
		Name: subj.Kind() + " services should have endpoints",
		Test: Eventually(func() error {
			for endpointName, addrCount := range map[string]int{
				subj.ServiceName(): int(subj.Count()),
			} {
				if addrCount == 0 {
					continue // maybe no test resource in this builder
				}
				endpoints, err := k.GetEndpoints(subj.NSN().Namespace, endpointName)
				if err != nil {
					return err
				}
				if len(endpoints.Subsets) == 0 {
					return fmt.Errorf("no subset for endpoint %s", endpointName)
				}
				if len(endpoints.Subsets[0].Addresses) != addrCount {
					return fmt.Errorf("%d addresses found for endpoint %s, expected %d", len(endpoints.Subsets[0].Addresses), endpointName, addrCount)
				}
			}
			return nil
		}),
	}
}

// StackMonitoringChecks tests that the monitored resource pods have 3 containers ready and that there are documents indexed in the beat indexes
// of the monitoring Elasticsearch clusters.
type StackMonitoringChecks struct {
	MonitoredNsn types.NamespacedName
	Metrics      elasticsearch.Builder
	Logs         elasticsearch.Builder
	K            *K8sClient
}

func (c StackMonitoringChecks)  Steps() StepList {
	return StepList{
		c.CheckBeatSidecars(),
		c.CheckMetricbeatIndex(),
		c.CheckFilebeatIndex(),
	}
}

func (c StackMonitoringChecks) CheckBeatSidecars() Step {
	return Step{
		Name: "Check that beat sidecars are running",
		Test: Eventually(func() error {
			pods, err := c.K.GetPods(ESPodListOptions(c.MonitoredNsn.Namespace, c.MonitoredNsn.Name)...)
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

func (c StackMonitoringChecks) CheckMetricbeatIndex() Step {
	return Step{
		Name: "Check that documents are indexed in one metricbeat-* index",
		Test: Eventually(func() error {
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

func (c StackMonitoringChecks) CheckFilebeatIndex() Step {
	return Step{
		Name: "Check that documents are indexed in one filebeat-* index",
		Test: Eventually(func() error {
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
	if docsCount < 0 {
		return fmt.Errorf("index [%s] empty", indexPattern)
	}

	return nil
}