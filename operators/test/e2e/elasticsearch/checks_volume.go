// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"testing"

	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/stretchr/testify/require"
)

func usesEmptyDir(es estype.Elasticsearch) bool {
	var emptyDirUsed bool
	for _, n := range es.Spec.Nodes {
		for _, v := range n.PodTemplate.Spec.Volumes {
			if v.EmptyDir != nil && v.Name == volume.ElasticsearchDataVolumeName {
				emptyDirUsed = true
			}
		}
	}
	return emptyDirUsed
}

func CheckESDataVolumeType(es estype.Elasticsearch, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Elasticsearch data volumes should be of the specified type",
		Test: func(t *testing.T) {
			checkForEmptyDir := usesEmptyDir(es)
			pods, err := k.GetPods(helpers.ESPodListOptions(es.Name))
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
