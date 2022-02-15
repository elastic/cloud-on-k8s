// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

func clusterGeneration(es esv1.Elasticsearch, k *test.K8sClient) (int64, error) {
	if err := k.Client.Get(context.Background(), types.NamespacedName{Namespace: es.GetNamespace(), Name: es.GetName()}, &es); err != nil {
		return 0, err
	}

	return es.GetObjectMeta().GetGeneration(), nil
}

func clusterObservedGeneration(es esv1.Elasticsearch, k *test.K8sClient) (int64, error) {
	if err := k.Client.Get(context.Background(), types.NamespacedName{Namespace: es.GetNamespace(), Name: es.GetName()}, &es); err != nil {
		return 0, err
	}

	return es.Status.ObservedGeneration, nil
}

// RetrieveClusterGenerationsStep stores the current metadata.Generation, and status.ObservedGeneration into the given fields.
func RetrieveClusterGenerationsStep(es esv1.Elasticsearch, k *test.K8sClient, generation, observedGeneration *int64) test.Step {
	return test.Step{
		Name: "Retrieve Elasticsearch metadata.Generation, and status.ObservedGeneration for comparison purpose",
		Test: test.Eventually(func() error {
			clusterGeneration, err := clusterGeneration(es, k)
			if err != nil {
				return err
			}
			*generation = clusterGeneration
			clusterObservedGeneration, err := clusterObservedGeneration(es, k)
			if err != nil {
				return err
			}
			*observedGeneration = clusterObservedGeneration
			return nil
		}),
	}
}

// CompareClusterGenerations compares the current clusterUUID with previousClusterUUID,
// and fails if they don't match
func CompareClusterGenerations(es esv1.Elasticsearch, k *test.K8sClient, previousClusterGeneration, previousClusterObservedGeneration *int64) test.Step {
	//nolint:thelper
	return test.Step{
		Name: "Cluster metadata.generation, and status.observedGeneration should have been incremented from previous state, and should be equal",
		Test: func(t *testing.T) {
			newClusterGeneration, err := clusterGeneration(es, k)
			require.NoError(t, err)
			require.NotEmpty(t, newClusterGeneration)
			newClusterObservedGeneration, err := clusterObservedGeneration(es, k)
			require.NoError(t, err)
			require.NotEmpty(t, newClusterObservedGeneration)
			require.Greater(t, newClusterGeneration, *previousClusterGeneration)
			require.Greater(t, newClusterObservedGeneration, *previousClusterObservedGeneration)
			require.Equal(t, newClusterGeneration, newClusterObservedGeneration)
		},
	}
}
