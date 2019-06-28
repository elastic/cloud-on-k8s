// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"context"
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/restart"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework/elasticsearch"
	"github.com/stretchr/testify/require"
)

func TestCoordinatedClusterRestart(t *testing.T) {
	b := elasticsearch.NewBuilder("test-restart").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	// keep track of nodes start time before the restart
	// it is supposed to be different after the restart is over
	var initialStartTime map[string]int64

	framework.Run(t, func(k *framework.K8sClient) framework.TestStepList {
		restartSteps := framework.TestStepList{
			framework.TestStep{
				Name: "Retrieve nodes start time",
				Test: func(t *testing.T) {
					startTime, err := getNodesStartTime(k, b.Elasticsearch)
					require.NoError(t, err)
					initialStartTime = startTime
				},
			},
			framework.TestStep{
				Name: "Nodes start time should stay the same if not restarted",
				Test: func(t *testing.T) {
					startTime, err := getNodesStartTime(k, b.Elasticsearch)
					require.NoError(t, err)
					require.Equal(t, initialStartTime, startTime)
				},
			},
			framework.TestStep{
				Name: "Annotate the cluster to schedule a restart",
				Test: func(t *testing.T) {
					// retrieve current cluster resource
					var cluster v1alpha1.Elasticsearch
					err := k.Client.Get(k8s.ExtractNamespacedName(&b.Elasticsearch), &cluster)
					require.NoError(t, err)
					// annotate it to have the operator schedule a restart
					restart.AnnotateClusterForCoordinatedRestart(&cluster)
					err = k.Client.Update(&cluster)
					require.NoError(t, err)
				},
			},
			framework.TestStep{
				// It's technically possible to detect pods becoming not ready during the restart,
				// but it might happen too fast for us to notice. Let's check the JVM start time instead.
				Name: "Wait for all nodes start time to have changed (restart is complete)",
				Test: framework.Eventually(func() error {
					// retrieve current start time
					startTime, err := getNodesStartTime(k, b.Elasticsearch)
					if err != nil {
						return err
					}
					// compare with initial start time
					for name, start := range startTime {
						initial, ok := initialStartTime[name]
						if !ok {
							return fmt.Errorf("node %s does not appear in the initial nodes", name)
						}
						if initial == start {
							return fmt.Errorf("node %s start time has not evolved yet: %d", name, start)
						}
					}
					return nil
				}),
			},
		}
		return append(restartSteps, framework.CheckTestSteps(b, k)...)
	}, b)
}

func getNodesStartTime(k *framework.K8sClient, cluster v1alpha1.Elasticsearch) (map[string]int64, error) {
	// retrieve nodes information
	esClient, err := elasticsearch.NewElasticsearchClient(cluster, k)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
	defer cancel()
	nodes, err := esClient.GetNodes(ctx)
	if err != nil {
		return nil, err
	}

	// we should retrieve data for all nodes
	if int32(len(nodes.Nodes)) != cluster.Spec.NodeCount() {
		return nil, fmt.Errorf("expected %d nodes, got %d", cluster.Spec.NodeCount(), len(nodes.Nodes))
	}

	// retrieve start time (in milliseconds) for each node
	startTime := make(map[string]int64, len(nodes.Nodes))
	for name, info := range nodes.Nodes {
		startTime[name] = info.JVM.StartTimeInMillis
	}

	return startTime, nil
}
