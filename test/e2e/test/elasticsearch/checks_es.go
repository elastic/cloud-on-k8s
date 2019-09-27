// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	estype "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"k8s.io/apimachinery/pkg/api/resource"
)

type esClusterChecks struct {
	es estype.Elasticsearch
	k  *test.K8sClient
}

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	e := esClusterChecks{
		es: b.Elasticsearch,
		k:  k,
	}
	return test.StepList{
		e.CheckESReachable(),
		e.CheckESVersion(b.Elasticsearch),
		e.CheckESHealthGreen(),
		e.CheckESNodesTopology(b.Elasticsearch),
	}
}

func (e *esClusterChecks) newESClient() (client.Client, error) {
	// recreate ES client for tests that switch between TlS/no TLS
	return NewElasticsearchClient(e.es, e.k)
}

func (e *esClusterChecks) CheckESReachable() test.Step {
	return test.Step{
		Name: "ES cluster health endpoint should eventually be reachable",
		Test: test.Eventually(func() error {
			ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
			defer cancel()
			esClient, err := e.newESClient()
			if err != nil {
				return err
			}
			if _, err := esClient.GetClusterHealth(ctx); err != nil {
				return err
			}
			return nil
		}),
	}
}

func (e *esClusterChecks) CheckESVersion(es estype.Elasticsearch) test.Step {
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
			actualHealth := estype.ElasticsearchHealth(health.Status)
			expectedHealth := estype.ElasticsearchGreenHealth
			if actualHealth != expectedHealth {
				return fmt.Errorf("cluster health is not green, but %s", actualHealth)
			}
			return nil
		}),
	}
}

func (e *esClusterChecks) CheckESNodesTopology(es estype.Elasticsearch) test.Step {
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
			var expectedTopology []estype.NodeSpec
			for _, node := range es.Spec.Nodes {
				for i := 0; i < int(node.NodeCount); i++ {
					expectedTopology = append(expectedTopology, node)
				}
			}
			// match each actual node to an expected node
			for nodeID, node := range nodes.Nodes {
				// check if node present is coming from the expected stateful set based on its name
				found := false
				for _, spec := range es.Spec.Nodes {
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
					cfg, err := v1beta1.UnpackConfig(topoElem.Config)
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

func rolesToConfig(roles []string) estype.Node {
	node := estype.Node{
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

func compareMemoryLimit(topologyElement estype.NodeSpec, cgroupMemoryLimitsInBytes int64) bool {
	var memoryLimit *resource.Quantity
	for _, c := range topologyElement.PodTemplate.Spec.Containers {
		if c.Name == v1beta1.ElasticsearchContainerName {
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
