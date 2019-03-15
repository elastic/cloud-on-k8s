// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	"github.com/stretchr/testify/assert"
)

type esClusterChecks struct {
	client *client.Client
}

// ESClusterChecks returns all test steps to verify the given stack's Elasticsearch
// cluster is running as expected
func ESClusterChecks(es estype.Elasticsearch, k *helpers.K8sHelper) helpers.TestStepList {
	e := esClusterChecks{}
	return helpers.TestStepList{
		e.BuildESClient(es, k),
		e.CheckESReachable(),
		e.CheckESVersion(es),
		e.CheckESLicense(es),
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
			if _, err := e.client.GetClusterHealth(context.TODO()); err != nil {
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
			info, err := e.client.GetClusterInfo(context.TODO())
			require.NoError(t, err)
			require.Equal(t, es.Spec.Version, info.Version.Number)
		},
	}
}

func (e *esClusterChecks) CheckESLicense(es estype.Elasticsearch) helpers.TestStep {
	return helpers.TestStep{
		Name: "Elasticsearch license type should be the expected one",
		Test: func(t *testing.T) {
			license, err := e.client.GetLicense(context.TODO())
			require.NoError(t, err)
			assert.Equal(t, es.Spec.GetLicenseType().String(), license.Type)
			assert.Equal(t, "active", license.Status)
		},
	}
}

func (e *esClusterChecks) CheckESHealthGreen() helpers.TestStep {
	return helpers.TestStep{
		Name: "Elasticsearch endpoint should eventually be reachable",
		Test: helpers.Eventually(func() error {
			health, err := e.client.GetClusterHealth(context.TODO())
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
		Name: "Elasticsearch nodes topology should be the expected one",
		Test: func(t *testing.T) {
			nodes, err := e.client.GetNodes(context.TODO())
			require.NoError(t, err)
			require.Equal(t, int(es.Spec.NodeCount()), len(nodes.Nodes))

			// flatten the topology
			var expectedTopology []estype.TopologyElementSpec
			for _, topoElem := range es.Spec.Topology {
				for i := 0; i < int(topoElem.NodeCount); i++ {
					expectedTopology = append(expectedTopology, topoElem)
				}
			}
			// match each actual node to an expected node
			for _, node := range nodes.Nodes {
				nodeTypes := rolesToNodeTypes(node.Roles)
				for i, topoElem := range expectedTopology {
					if topoElem.NodeTypes == nodeTypes && compareMemoryLimit(topoElem, node.JVM.Mem.HeapMaxInBytes) {
						// it's a match! #tinder
						// no need to match this topology anymore
						expectedTopology = append(expectedTopology[:i], expectedTopology[i+1:]...)
						break
					}
				}
			}
			// expected topology should have matched all nodes
			require.Empty(t, expectedTopology)
		},
	}
}

func rolesToNodeTypes(roles []string) estype.NodeTypesSpec {
	nt := estype.NodeTypesSpec{}
	for _, r := range roles {
		switch r {
		case "master":
			nt.Master = true
		case "data":
			nt.Data = true
		case "ingest":
			nt.Ingest = true
		case "ml":
			nt.ML = true
		}
	}
	return nt
}

func compareMemoryLimit(topologyElement estype.TopologyElementSpec, heapMaxBytes int) bool {
	if topologyElement.Resources.Limits.Memory() == nil {
		// no expected memory, consider it's ok
		return true
	}

	const epsilon = 0.05 // allow a 5% diff due to bytes approximation

	expectedBytes := topologyElement.Resources.Limits.Memory().Value()
	actualBytes := int64(heapMaxBytes * 2) // we set heap to half the available memory

	diffRatio := math.Abs(float64(actualBytes-expectedBytes)) / math.Abs(float64(expectedBytes))
	if diffRatio < epsilon {
		return true
	}
	return false
}
