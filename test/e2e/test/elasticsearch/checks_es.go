// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"k8s.io/apimachinery/pkg/api/resource"
)

type esClusterChecks struct {
	es esv1.Elasticsearch
	k  *test.K8sClient
}

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	e := esClusterChecks{
		es: b.Elasticsearch,
		k:  k,
	}
	return test.StepList{
		e.CheckESNodesTopology(b.Elasticsearch),
		e.CheckESVersion(b.Elasticsearch),
		e.CheckESHealthGreen(),
	}
}

func (e *esClusterChecks) newESClient() (client.Client, error) {
	// recreate ES client for tests that switch between TlS/no TLS
	return NewElasticsearchClient(e.es, e.k)
}

func (e *esClusterChecks) CheckESVersion(es esv1.Elasticsearch) test.Step {
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
			if es.Spec.Version != info.Version.Number {
				return fmt.Errorf("expected %s, got %s", es.Spec.Version, info.Version.Number)
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

func (e *esClusterChecks) CheckESNodesTopology(es esv1.Elasticsearch) test.Step {
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

			v, err := version.Parse(es.Spec.Version)
			if err != nil {
				return err
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
				// check if node is coming from the expected stateful set based on its name,
				// ignore nodes coming from StatefulSets in the process of being downscaled
				found := false
				for _, spec := range es.Spec.NodeSets {
					if strings.Contains(node.Name, spec.Name) {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("none of spec names was found in %s", node.Name)
				}

				found = false
				for k := range nodesStats.Nodes {
					if k == nodeID {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("%s was not in %+v", nodeID, nodesStats.Nodes)
				}

				nodeStats := nodesStats.Nodes[nodeID]
				for i, topoElem := range expectedTopology {
					cfg := esv1.DefaultCfg(*v)
					if err := esv1.UnpackConfig(topoElem.Config, *v, &cfg); err != nil {
						return err
					}

					// ES returns a string, parse it as an int64, base10:
					cgroupMemoryLimitsInBytes, err := strconv.ParseInt(
						nodeStats.OS.CGroup.Memory.LimitInBytes, 10, 64,
					)
					if err != nil {
						return err
					}

					if compareRoles(cfg.Node, node.Roles) &&
						compareMemoryLimit(topoElem, cgroupMemoryLimitsInBytes) {
						// no need to match this topology anymore
						expectedTopology = append(expectedTopology[:i], expectedTopology[i+1:]...)
						break
					}
				}
			}
			// expected topology should have matched all nodes
			if len(expectedTopology) > 0 {
				return fmt.Errorf("expected elements missing from cluster %+v", expectedTopology)
			}
			return nil
		}),
	}
}

func compareRoles(expected *esv1.Node, actualRoles []string) bool {
	for _, r := range actualRoles {
		switch r {
		case "master":
			if !expected.HasMasterRole() {
				return false
			}
		case "data":
			if !expected.HasDataRole() {
				return false
			}
		case "ingest":
			if !expected.HasIngestRole() {
				return false
			}
		case "transform":
			if !expected.HasTransformRole() {
				return false
			}
		}
	}
	return true
}

func compareMemoryLimit(topologyElement esv1.NodeSet, cgroupMemoryLimitsInBytes int64) bool {
	var memoryLimit *resource.Quantity
	for _, c := range topologyElement.PodTemplate.Spec.Containers {
		if c.Name == esv1.ElasticsearchContainerName {
			memoryLimit = c.Resources.Limits.Memory()
		}
	}
	if memoryLimit == nil {
		// no expected memory, consider it's ok
		return true
	}

	expectedBytes := memoryLimit.Value()

	return expectedBytes == cgroupMemoryLimitsInBytes
}
