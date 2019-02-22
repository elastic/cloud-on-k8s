// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	"fmt"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	"github.com/stretchr/testify/require"
)

// RetrieveClusterUUIDStep stores the current clusterUUID into the given futureClusterUUID
func RetrieveClusterUUIDStep(es v1alpha1.ElasticsearchCluster, k *helpers.K8sHelper, futureClusterUUID *string) helpers.TestStep {
	return helpers.TestStep{
		Name: "Retrieve cluster UUID for comparison purpose",
		Test: helpers.Eventually(func() error {
			var e v1alpha1.ElasticsearchCluster
			err := k.Client.Get(k8s.ExtractNamespacedName(&es), &e)
			if err != nil {
				return err
			}
			clusterUUID := e.Status.ClusterUUID
			if clusterUUID == "" {
				return fmt.Errorf("Empty ClusterUUID")
			}
			*futureClusterUUID = clusterUUID
			return nil
		}),
	}
}

// CompareClusterUUIDStep compares the current clusterUUID with previousClusterUUID,
// and fails if they don't match
func CompareClusterUUIDStep(es v1alpha1.ElasticsearchCluster, k *helpers.K8sHelper, previousClusterUUID *string) helpers.TestStep {
	return helpers.TestStep{
		Name: "Cluster UUID should have been preserved",
		Test: func(t *testing.T) {
			var e v1alpha1.ElasticsearchCluster
			err := k.Client.Get(k8s.ExtractNamespacedName(&es), &e)
			require.NoError(t, err)
			newClusterUUID := e.Status.ClusterUUID
			require.NotEmpty(t, *previousClusterUUID)
			require.Equal(t, *previousClusterUUID, newClusterUUID)
		},
	}
}
