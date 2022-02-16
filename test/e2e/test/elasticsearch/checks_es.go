// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

type esClusterChecks struct {
	Builder
	k *test.K8sClient
}

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	e := esClusterChecks{b, k}
	return test.StepList{
		e.CheckESNodesTopology(),
		e.CheckESVersion(),
		e.CheckESHealthGreen(),
		e.CheckTransportCertificatesStep(),
	}
}

func (e *esClusterChecks) newESClient() (client.Client, error) {
	// recreate ES client for tests that switch between TlS/no TLS
	return NewElasticsearchClient(e.Elasticsearch, e.k)
}

func (e *esClusterChecks) CheckESVersion() test.Step {
	return test.Step{
		Name: "ES version should be the expected one",
		Test: test.Eventually(func() error {
			esClient, err := e.newESClient()
			if err != nil {
				return err
			}

			info, err := esClient.GetClusterInfo(context.Background())
			if err != nil {
				return err
			}
			if e.Elasticsearch.Spec.Version != info.Version.Number {
				return fmt.Errorf("expected %s, got %s", e.Elasticsearch.Spec.Version, info.Version.Number)
			}
			return nil
		}),
	}
}

func (e *esClusterChecks) CheckESHealthGreen() test.Step {
	return test.Step{
		Name: "ES endpoint should eventually be reachable",
		Test: test.Eventually(func() error {
			esClient, err := e.newESClient()
			if err != nil {
				return err
			}

			health, err := esClient.GetClusterHealth(context.Background())
			if err != nil {
				return err
			}
			actualHealth := health.Status
			expectedHealth := esv1.ElasticsearchGreenHealth
			if actualHealth != expectedHealth {
				return fmt.Errorf("cluster health is not green, but %s", actualHealth)
			}
			return nil
		}),
		OnFailure: printShardsAndAllocation(e.newESClient),
	}
}

func (e *esClusterChecks) CheckESNodesTopology() test.Step {
	return test.Step{
		Name: "ES nodes topology should eventually be the expected one",
		Test: test.Eventually(func() error {
			esClient, err := e.newESClient()
			if err != nil {
				return err
			}

			nodes, err := esClient.GetNodes(context.Background())
			if err != nil {
				return err
			}
			es := e.GetExpectedElasticsearch()
			if int(es.Spec.NodeCount()) != len(nodes.Nodes) {
				return fmt.Errorf("expected node count %d but was %d", es.Spec.NodeCount(), len(nodes.Nodes))
			}

			nodesStats, err := esClient.GetNodesStats(context.Background())
			if err != nil {
				return err
			}
			if int(es.Spec.NodeCount()) != len(nodesStats.Nodes) {
				return fmt.Errorf(
					"expected node count %d but _nodes/stats returned %d", es.Spec.NodeCount(), len(nodesStats.Nodes),
				)
			}

			// flatten the topology
			var expectedTopology []esv1.NodeSet
			for _, node := range es.Spec.NodeSets {
				for i := 0; i < int(node.Count); i++ {
					expectedTopology = append(expectedTopology, node)
				}
			}
			// match each actual node to an expected node
			for nodeID, node := range nodes.Nodes {
				found := false
				for k := range nodesStats.Nodes {
					if k == nodeID {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("%s was not in %+v", nodeID, nodesStats.Nodes)
				}

				// match the actual Elasticsearch node to an expected one in expectedTopology
				nodeStats := nodesStats.Nodes[nodeID]
				var err error
				var allErrors []string
				for i, topoElem := range expectedTopology {
					if err = e.compareTopology(es, topoElem, node, nodeStats); err == nil {
						// found it! no need to match this topology anymore
						expectedTopology = append(expectedTopology[:i], expectedTopology[i+1:]...)
						break
					}
					allErrors = append(allErrors, fmt.Sprintf("%s: %s", topoElem.Name, err.Error()))
				}
				if err != nil {
					// node reported from ES API does not match any expected node in the spec
					// (could be normal and transient on downscales)
					return fmt.Errorf("actual node %s in the cluster does not match any expected nodes: %v", node.Name, strings.Join(allErrors, ", "))
				}
			}
			// expected topology should have matched all nodes
			if len(expectedTopology) > 0 {
				return fmt.Errorf("%d expected elements missing from cluster %+v", len(expectedTopology), expectedTopology)
			}
			return nil
		}),
	}
}

func (e *esClusterChecks) compareTopology(es esv1.Elasticsearch, topoElem esv1.NodeSet, node client.Node, nodeStats client.NodeStats) error {
	// check if node is coming from the expected stateful set based on its name,
	// ignore nodes coming from StatefulSets in the process of being downscaled
	if !strings.Contains(node.Name, topoElem.Name) {
		return fmt.Errorf("node does not belong to nodeSet")
	}
	// get config to check roles
	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		return err
	}
	cfg := esv1.DefaultCfg(v)
	if err := esv1.UnpackConfig(topoElem.Config, v, &cfg); err != nil {
		return err
	}
	if err = compareRoles(cfg.Node, node.Roles); err != nil {
		return err
	}

	if err = compareCgroupMemoryLimit(topoElem, nodeStats); err != nil {
		return err
	}

	if err = compareCgroupCPULimit(topoElem, nodeStats); err != nil {
		return err
	}

	// get pods to check ressources requirements
	pods, err := e.k.GetPods(test.ESPodListOptionsByNodeSet(e.Elasticsearch.Namespace, e.Elasticsearch.Name, topoElem.Name)...)
	if err != nil {
		return err
	}
	if len(pods) == 0 {
		return fmt.Errorf("no pod found for ES %q / nodeSet %q", e.Elasticsearch.Name, topoElem.Name)
	}
	if err = compareSpecResources(topoElem, pods); err != nil {
		return err
	}

	return compareClaimedStorage(e.k, topoElem, pods)
}

func compareRoles(expected *esv1.Node, actualRoles []string) error {
	for _, r := range actualRoles {
		if !expected.IsConfiguredWithRole(esv1.NodeRole(r)) {
			return fmt.Errorf("actual role %s not expected", r)
		}
	}
	return nil
}

// compareCgroupMemoryLimit compares the memory limit specified in a nodeSet with the limit set in the memory control group at the OS level
func compareCgroupMemoryLimit(topologyElement esv1.NodeSet, nodeStats client.NodeStats) error {
	var memoryLimit *resource.Quantity
	for _, c := range topologyElement.PodTemplate.Spec.Containers {
		if c.Name == esv1.ElasticsearchContainerName {
			memoryLimit = c.Resources.Limits.Memory()
		}
	}
	if memoryLimit == nil || memoryLimit.IsZero() {
		// no expected memory, consider it's ok
		return nil
	}
	// ES returns a string, parse it as an int64, base10
	actualCgroupMemoryLimit, err := strconv.ParseInt(
		nodeStats.OS.CGroup.Memory.LimitInBytes, 10, 64,
	)
	if err != nil {
		return err
	}
	expectedCgroupMemoryLimit := memoryLimit.Value()
	if expectedCgroupMemoryLimit != actualCgroupMemoryLimit {
		return fmt.Errorf("expected cgroup memory limit %d, got %d", expectedCgroupMemoryLimit, actualCgroupMemoryLimit)
	}
	return nil
}

// compareCgroupCPULimit compares the CPU limit specified in a nodeSet with the limit set in the CPU control group at the OS level
func compareCgroupCPULimit(topologyElement esv1.NodeSet, nodeStats client.NodeStats) error {
	var expectedCPULimit *resource.Quantity
	for _, c := range topologyElement.PodTemplate.Spec.Containers {
		if c.Name == esv1.ElasticsearchContainerName {
			expectedCPULimit = c.Resources.Limits.Cpu()
		}
	}
	if expectedCPULimit == nil || expectedCPULimit.IsZero() {
		// no expected cpu, consider it's ok
		return nil
	}

	cgroupCPU := nodeStats.OS.CGroup.CPU
	actualCgroupCPULimit := float64(cgroupCPU.CFSQuotaMicros) / float64(cgroupCPU.CFSPeriodMicros)
	if expectedCPULimit.AsApproximateFloat64() != actualCgroupCPULimit {
		return fmt.Errorf("expected cgroup CPU limit [%f], got [%f]", expectedCPULimit.AsApproximateFloat64(), actualCgroupCPULimit)
	}
	return nil
}

// compareSpecResources compares the resources specified in the Elasticsearch resource with the resources
// specified in the pods
func compareSpecResources(topologyElement esv1.NodeSet, pods []corev1.Pod) error {
	var expected *corev1.ResourceRequirements
	for _, c := range topologyElement.PodTemplate.Spec.Containers {
		container := c
		if c.Name == esv1.ElasticsearchContainerName {
			expected = &container.Resources
		}
	}
	if expected == nil {
		expected = &nodespec.DefaultResources
	}

	for _, pod := range pods {
		for _, c := range pod.Spec.Containers {
			actual := c.Resources
			if c.Name == esv1.ElasticsearchContainerName { //nolint:nestif
				if expected.Requests != nil {
					if err := compareQuantity(pod.Name, "CPU request", expected.Requests.Cpu(), actual.Requests.Cpu()); err != nil {
						return err
					}
					if err := compareQuantity(pod.Name, "memory request", expected.Requests.Memory(), actual.Requests.Memory()); err != nil {
						return err
					}
				}
				if expected.Limits != nil {
					if err := compareQuantity(pod.Name, "CPU limit", expected.Limits.Cpu(), actual.Limits.Cpu()); err != nil {
						return err
					}
					if err := compareQuantity(pod.Name, "memory limit", expected.Limits.Memory(), actual.Limits.Memory()); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func compareQuantity(podName string, resourceType string, expected, actual *resource.Quantity) error {
	if !expected.IsZero() && !equality.Semantic.DeepEqual(expected, actual) {
		return fmt.Errorf("pod [%s] expected %s [%d], got [%d]", podName, resourceType, expected.Value(), actual.Value())
	}
	return nil
}

// compareClaimedStorage compares the requested storage specified in a nodeSet with the actual capacity claimed in the PVC
func compareClaimedStorage(k8sClient *test.K8sClient, topologyElement esv1.NodeSet, pods []corev1.Pod) error {
	var expectedStorage *resource.Quantity
	for _, v := range topologyElement.VolumeClaimTemplates {
		if v.Name == volume.ElasticsearchDataVolumeName {
			expectedStorage = v.Spec.Resources.Requests.Storage()
		}
	}
	if expectedStorage == nil {
		// no expected storage, consider it's ok
		return nil
	}
	pvcs, err := k8sClient.GetPVCsByPods(pods)
	if err != nil {
		return err
	}
	if len(pods) != len(pvcs) {
		return fmt.Errorf("expected PVC count %q, got %q", len(pods), len(pvcs))
	}
	for _, pvc := range pvcs {
		actualStorage := pvc.Spec.Resources.Requests.Storage()
		if !equality.Semantic.DeepEqual(expectedStorage, actualStorage) {
			return fmt.Errorf("pvc [%s] expected claimed storage [%d], got [%d]", pvc.Name, expectedStorage.Value(), actualStorage.Value())
		}
	}
	return nil
}
