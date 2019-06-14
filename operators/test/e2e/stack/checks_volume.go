/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package stack

import (
	"testing"

	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/stretchr/testify/require"
)

func CheckDefaultPVC(es estype.Elasticsearch, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Elasticsearch data volumes should be using defaulted PVCs",
		Test: func(t *testing.T) {
			pods, err := k.GetPods(helpers.ESPodListOptions(es.Name))
			require.NoError(t, err)
			for _, p := range pods {
				for _, v := range p.Spec.Volumes {
					if v.Name != volume.ElasticsearchDataVolumeName {
						continue
					}
					require.Nil(t, v.EmptyDir)
					require.NotNil(t, v.PersistentVolumeClaim)
				}
			}
		},
	}
}

func CheckEmptyDir(es estype.Elasticsearch, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Elasticsearch data volumes should be emptyDirs",
		Test: func(t *testing.T) {
			pods, err := k.GetPods(helpers.ESPodListOptions(es.Name))
			require.NoError(t, err)
			for _, p := range pods {
				for _, v := range p.Spec.Volumes {
					if v.Name != volume.ElasticsearchDataVolumeName {
						continue
					}
					require.Nil(t, v.PersistentVolumeClaim)
					require.NotNil(t, v.EmptyDir)
				}
			}
		},
	}
}
