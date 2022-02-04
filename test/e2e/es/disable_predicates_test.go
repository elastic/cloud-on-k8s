// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e
// +build es e2e

package es

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
)

func TestRedClusterCanBeModifiedByDisablingPredicate(t *testing.T) {
	podTemplate1 := elasticsearch.ESPodTemplate(elasticsearch.DefaultResources)
	podTemplate1.Annotations = map[string]string{"foo": "bar"}

	k := test.NewK8sClientOrFatal()
	initial := elasticsearch.NewBuilder("test-v-up-with-red-cluster-7x").
		WithNodeSet(esv1.NodeSet{
			Name:        "default",
			Count:       1,
			PodTemplate: podTemplate1,
		}).
		WithAnnotation(esv1.DisableUpgradePredicatesAnnotation, "only_restart_healthy_node_if_green_or_yellow")

	podTemplate2 := elasticsearch.ESPodTemplate(elasticsearch.DefaultResources)
	podTemplate2.Annotations = map[string]string{"foo": "bar2"}

	mutated := initial.WithNoESTopology().
		WithNodeSet(esv1.NodeSet{
			Name:        "default",
			Count:       1,
			PodTemplate: podTemplate2,
		}).
		WithAnnotation(esv1.DisableUpgradePredicatesAnnotation, "only_restart_healthy_node_if_green_or_yellow")

	var esClient client.Client

	elasticsearch.ForcedUpgradeTestStepsWithPostSteps(
		k,
		initial,
		[]test.Step{
			// wait for the cluster to become green
			elasticsearch.CheckClusterHealth(initial, k),
			{
				Name: "Create ES client",
				Test: func(t *testing.T) {
					var err error
					esClient, err = elasticsearch.NewElasticsearchClient(initial.Elasticsearch, k)
					require.NoError(t, err)
				},
			},
			{
				Name: "Misconfigure index on cluster, turning cluster red",
				Test: func(t *testing.T) {
					settings := `
{
    "settings": {
		"index.routing.allocation.include._id": "does not exist"
    }
}
`
					r, err := http.NewRequest(http.MethodPut, "/test-index", bytes.NewBufferString(settings))
					require.NoError(t, err)
					response, err := esClient.Request(context.Background(), r)
					defer response.Body.Close() // nolint
					require.NoError(t, err)
				},
			},
			// wait for the cluster to become red
			elasticsearch.CheckSpecificClusterHealth(initial, k, esv1.ElasticsearchRedHealth),
		},
		mutated,
		[]test.Step{
			{
				Name: "Delete misconfigured index on cluster, allowing cluster to turn back green",
				Test: func(t *testing.T) {
					r, err := http.NewRequest(http.MethodDelete, "/test-index", nil)
					require.NoError(t, err)
					response, err := esClient.Request(context.Background(), r)
					defer response.Body.Close() // nolint
					require.NoError(t, err)
				},
			},
		},
	).RunSequential(t)
}
