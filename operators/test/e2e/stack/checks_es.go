// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
)

type esClusterChecks struct {
	client client.Client
}

// ESClusterChecks returns all test steps to verify the given stack's Elasticsearch
// cluster is running as expected
func ESClusterChecks(es estype.Elasticsearch, k *helpers.K8sHelper) helpers.TestStepList {
	e := esClusterChecks{}
	return helpers.TestStepList{
		e.BuildESClient(es, k),
		e.CheckESReachable(),
		e.CheckESVersion(es),
		e.CheckESHealthGreen(),
		e.CheckESNodesTopology(es),
	}
}

func (e *esClusterChecks) BuildESClient(es estype.Elasticsearch, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Every secret should be set so that we can build an ES client",
		Test: func(t *testing.T) {
			esClient, err := helpers.NewElasticsearchClient(es, k)
			assert.NoError(t, err)
			e.client = esClient
		},
	}
}

func (e *esClusterChecks) CheckESReachable() helpers.TestStep {
	return helpers.TestStep{
		Name: "Elasticsearch endpoint should eventually be reachable",
		Test: helpers.Eventually(func() error {
			ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
			defer cancel()
			if _, err := e.client.GetClusterHealth(ctx); err != nil {
				return err
			}
			return nil
		}),
	}
}

func (e *esClusterChecks) CheckESVersion(es estype.Elasticsearch) helpers.TestStep {
	return helpers.TestStep{
		Name: "Elasticsearch version should be the expected one",
		Test: func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
			defer cancel()
			info, err := e.client.GetClusterInfo(ctx)
			require.NoError(t, err)
			require.Equal(t, es.Spec.Version, info.Version.Number)
		},
	}
}

func (e *esClusterChecks) CheckESHealthGreen() helpers.TestStep {
	return helpers.TestStep{
		Name: "Elasticsearch endpoint should eventually be reachable",
		Test: helpers.Eventually(func() error {
			ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
			defer cancel()
			health, err := e.client.GetClusterHealth(ctx)
			if err != nil {
				return err
			}
			actualHealth := estype.ElasticsearchHealth(health.Status)
			expectedHealth := estype.ElasticsearchGreenHealth
			if actualHealth != expectedHealth {
				return fmt.Errorf("Cluster health is not green, but %s", actualHealth)
			}
			return nil
		}),
	}
}

func (e *esClusterChecks) CheckESNodesTopology(es estype.Elasticsearch) helpers.TestStep {
	return helpers.TestStep{
		Name: "Elasticsearch nodes topology should eventually be the expected one",
		Test: helpers.Eventually(func() error {
			ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
			defer cancel()

			nodes, err := e.client.GetNodes(ctx)
			if err != nil {
				return err
			}
			if int(es.Spec.NodeCount()) != len(nodes.Nodes) {
				return fmt.Errorf("expected node count %d but was %d", es.Spec.NodeCount(), len(nodes.Nodes))
			}

			nodesStats, err := e.client.GetNodesStats(ctx)
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
			for nodeId, node := range nodes.Nodes {
				nodeRoles := rolesToConfig(node.Roles)

				var found bool
				for k, _ := range nodesStats.Nodes {
					if k == nodeId {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("%s was not in %+v", nodeId, nodesStats.Nodes)
				}

				nodeStats := nodesStats.Nodes[nodeId]
				for i, topoElem := range expectedTopology {
					cfg, err := v1alpha1.UnpackConfig(topoElem.Config)
					if err != nil {
						return err
					}

					podNameExample := name.NewPodName(es.Name, topoElem)

					// ES returns a string, parse it as an int64, base10:
					cgroupMemoryLimitsInBytes, err := strconv.ParseInt(
						nodeStats.OS.CGroup.Memory.LimitInBytes, 10, 64,
					)
					if err != nil {
						return err
					}

					if cfg.Node == nodeRoles &&
						compareMemoryLimit(topoElem, cgroupMemoryLimitsInBytes) &&
						// compare the base names of the pod and topology to ensure they're from the same nodespec
						name.Basename(node.Name) == name.Basename(podNameExample) {
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
		if c.Name == v1alpha1.ElasticsearchContainerName {
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
