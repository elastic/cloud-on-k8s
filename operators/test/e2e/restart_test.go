// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/restart"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	"github.com/elastic/k8s-operators/operators/test/e2e/stack"
)

func TestCoordinatedClusterRestart(t *testing.T) {
	k := helpers.NewK8sClientOrFatal()
	s := stack.NewStackBuilder("test-restart").
		WithESMasterDataNodes(3, stack.DefaultResources)

	// keep track of nodes start time before the restart
	// it is supposed to be different after the restart is over
	var initialStartTime map[string]int64

	helpers.TestStepList{}.
		// create the cluster
		WithSteps(stack.InitTestSteps(s, k)...).
		WithSteps(stack.CreationTestSteps(s, k)...).
		WithSteps(
			helpers.TestStep{
				Name: "Retrieve nodes start time",
				Test: func(t *testing.T) {
					startTime, err := getNodesStartTime(k, s.Elasticsearch)
					require.NoError(t, err)
					initialStartTime = startTime
				},
			},
			helpers.TestStep{
				Name: "Nodes start time should stay the same if not restarted",
				Test: func(t *testing.T) {
					startTime, err := getNodesStartTime(k, s.Elasticsearch)
					require.NoError(t, err)
					require.Equal(t, initialStartTime, startTime)
				},
			},
			helpers.TestStep{
				Name: "Annotate the cluster to schedule a restart",
				Test: func(t *testing.T) {
					// retrieve current cluster resource
					var cluster v1alpha1.Elasticsearch
					err := k.Client.Get(k8s.ExtractNamespacedName(&s.Elasticsearch), &cluster)
					require.NoError(t, err)
					// annotate it to have the operator schedule a restart
					restart.AnnotateClusterForCoordinatedRestart(&cluster)
					err = k.Client.Update(&cluster)
					require.NoError(t, err)
				},
			},
			helpers.TestStep{
				// It's technically possible to detect pods becoming not ready during the restart,
				// but it might happen too fast for us to notice. Let's check the JVM start time instead.
				Name: "Wait for all nodes start time to have changed (restart is complete)",
				Test: helpers.Eventually(func() error {
					// retrieve current start time
					startTime, err := getNodesStartTime(k, s.Elasticsearch)
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
		).
		// we should get back to a green cluster
		WithSteps(stack.CheckStackSteps(s, k)...).
		// finally, cleanup resources
		WithSteps(stack.DeletionTestSteps(s, k)...).
		RunSequential(t)
}

func getNodesStartTime(k *helpers.K8sHelper, cluster v1alpha1.Elasticsearch) (map[string]int64, error) {
	// retrieve nodes information
	esClient, err := helpers.NewElasticsearchClient(cluster, k)
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
