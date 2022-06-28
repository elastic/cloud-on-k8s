// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"testing"

	"github.com/stretchr/testify/require"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func usesEmptyDir(es esv1.Elasticsearch) bool {
	var emptyDirUsed bool
	for _, n := range es.Spec.NodeSets {
		for _, v := range n.PodTemplate.Spec.Volumes {
			if v.EmptyDir != nil && v.Name == volume.ElasticsearchDataVolumeName {
				emptyDirUsed = true
			}
		}
	}
	return emptyDirUsed
}

func CheckESDataVolumeType(es esv1.Elasticsearch, k *test.K8sClient) test.Step {
	//nolint:thelper
	return test.Step{
		Name: "Elasticsearch data volumes should be of the specified type",
		Test: func(t *testing.T) {
			checkForEmptyDir := usesEmptyDir(es)
			pods, err := k.GetPods(test.ESPodListOptions(es.Namespace, es.Name)...)
			require.NoError(t, err)
			for _, p := range pods {
				for _, v := range p.Spec.Volumes {
					if v.Name != volume.ElasticsearchDataVolumeName {
						continue
					}
					if checkForEmptyDir {
						require.Nil(t, v.PersistentVolumeClaim)
						require.NotNil(t, v.EmptyDir)
					} else {
						require.Nil(t, v.EmptyDir)
						require.NotNil(t, v.PersistentVolumeClaim)
					}
				}
			}
		},
	}
}
