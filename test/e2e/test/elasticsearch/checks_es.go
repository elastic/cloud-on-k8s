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
			ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
			defer cancel()
			info, err := esClient.GetClusterInfo(ctx)
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
			ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
			defer cancel()
			health, err := esClient.GetClusterHealth(ctx)
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
			ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
			defer cancel()

			nodes, err := esClient.GetNodes(ctx)
			if err != nil {
				return err
			}
			if int(es.Spec.NodeCount()) != len(nodes.Nodes) {
				return fmt.Errorf("expected node count %d but was %d", es.Spec.NodeCount(), len(nodes.Nodes))
			}

			nodesStats, err := esClient.GetNodesStats(ctx)
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

				nodeRoles := rolesToConfig(node.Roles)
				nodeStats := nodesStats.Nodes[nodeID]
				for i, topoElem := range expectedTopology {
					cfg, err := esv1.UnpackConfig(topoElem.Config)
					if err != nil {
						return err
					}

					// ES returns a string, parse it as an int64, base10:
					cgroupMemoryLimitsInBytes, err := strconv.ParseInt(
						nodeStats.OS.CGroup.Memory.LimitInBytes, 10, 64,
					)
					if err != nil {
						return err
					}

					if cfg.Node == nodeRoles &&
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

func rolesToConfig(roles []string) esv1.Node {
	node := esv1.Node{
		ML: true, // ML is not reported in roles array, we assume true
	}
	for _, r := range roles {
		switch r {
		case "master":
			node.Master = true
		case "data":
			node.Data = true
		case "ingest":
			node.Ingest = true
		}
	}
	return node
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
