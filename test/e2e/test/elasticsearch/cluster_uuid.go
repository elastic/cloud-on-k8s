// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"fmt"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/stretchr/testify/require"
)

func clusterUUID(es esv1.Elasticsearch, k *test.K8sClient) (string, error) {
	client, err := NewElasticsearchClient(es, k)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()
	info, err := client.GetClusterInfo(ctx)
	if err != nil {
		return "", err
	}
	return info.ClusterUUID, nil
}

// RetrieveClusterUUIDStep stores the current clusterUUID into the given futureClusterUUID
func RetrieveClusterUUIDStep(es esv1.Elasticsearch, k *test.K8sClient, futureClusterUUID *string) test.Step {
	return test.Step{
		Name: "Retrieve Elasticsearch cluster UUID for comparison purpose",
		Test: test.Eventually(func() error {
			uuid, err := clusterUUID(es, k)
			if err != nil {
				return err
			}
			clusterUUID := uuid
			if clusterUUID == "_na_" {
				return fmt.Errorf("cluster still forming")
			}
			*futureClusterUUID = clusterUUID
			return nil
		}),
	}
}

// CompareClusterUUIDStep compares the current clusterUUID with previousClusterUUID,
// and fails if they don't match
func CompareClusterUUIDStep(es esv1.Elasticsearch, k *test.K8sClient, previousClusterUUID *string) test.Step {
	return test.Step{
		Name: "Cluster UUID should have been preserved",
		Test: func(t *testing.T) {
			newClusterUUID, err := clusterUUID(es, k)
			require.NoError(t, err)
			require.NotEmpty(t, *previousClusterUUID)
			require.Equal(t, *previousClusterUUID, newClusterUUID)
		},
	}
}
