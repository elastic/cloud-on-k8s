// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"errors"
	"fmt"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func clusterUUID(es esv1.Elasticsearch, k *test.K8sClient) (string, error) {
	client, err := NewElasticsearchClient(es, k)
	if err != nil {
		return "", err
	}

	info, err := client.GetClusterInfo(context.Background())
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
		Test: test.Eventually(func() error {
			newClusterUUID, err := clusterUUID(es, k)
			if err != nil {
				return err
			}
			if previousClusterUUID == nil || *previousClusterUUID == "" {
				return errors.New("test setup error previousClusterUUID is empty or nil")
			}
			if *previousClusterUUID != newClusterUUID {
				return fmt.Errorf("cluster state lost, prev cluster UUID %s, current %s", *previousClusterUUID, newClusterUUID)
			}
			return nil
		}),
	}
}
