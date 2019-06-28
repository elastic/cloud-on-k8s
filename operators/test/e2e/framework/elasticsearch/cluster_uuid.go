// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework"
	"github.com/stretchr/testify/require"
)

// RetrieveClusterUUIDStep stores the current clusterUUID into the given futureClusterUUID
func RetrieveClusterUUIDStep(es v1alpha1.Elasticsearch, k *framework.K8sClient, futureClusterUUID *string) framework.TestStep {
	return framework.TestStep{
		Name: "Retrieve Elasticsearch cluster UUID for comparison purpose",
		Test: framework.Eventually(func() error {
			var e v1alpha1.Elasticsearch
			err := k.Client.Get(k8s.ExtractNamespacedName(&es), &e)
			if err != nil {
				return err
			}
			clusterUUID := e.Status.ClusterUUID
			if clusterUUID == "" {
				return fmt.Errorf("empty ClusterUUID")
			}
			*futureClusterUUID = clusterUUID
			return nil
		}),
	}
}

// CompareClusterUUIDStep compares the current clusterUUID with previousClusterUUID,
// and fails if they don't match
func CompareClusterUUIDStep(es v1alpha1.Elasticsearch, k *framework.K8sClient, previousClusterUUID *string) framework.TestStep {
	return framework.TestStep{
		Name: "Cluster UUID should have been preserved",
		Test: func(t *testing.T) {
			var e v1alpha1.Elasticsearch
			err := k.Client.Get(k8s.ExtractNamespacedName(&es), &e)
			require.NoError(t, err)
			newClusterUUID := e.Status.ClusterUUID
			require.NotEmpty(t, *previousClusterUUID)
			require.Equal(t, *previousClusterUUID, newClusterUUID)
		},
	}
}
