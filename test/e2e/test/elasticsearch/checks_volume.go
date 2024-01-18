// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"testing"

	"github.com/stretchr/testify/require"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
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

func CheckStackConfigPolicyESSecretMountsVolume(k *test.K8sClient, es esv1.Elasticsearch, scp policyv1alpha1.StackConfigPolicy) test.Step {
	//nolint:thelper
	return test.Step{
		Name: "Stack Config Policy Elasticsearch Secret Mounts should be present in Pod",
		Test: func(t *testing.T) {
			pods, err := k.GetPods(test.ESPodListOptions(es.Namespace, es.Name)...)
			require.NoError(t, err)
			for _, p := range pods {
				volumeMountPathMap := make(map[string]string)
				for _, c := range p.Spec.Containers {
					if c.Name == "elasticsearch" {
						for _, volumeMount := range c.VolumeMounts {
							volumeMountPathMap[volumeMount.Name] = volumeMount.MountPath
						}
					}
				}
				// Make sure the secret name and the mountpath match
				for _, secretMount := range scp.Spec.Elasticsearch.SecretMounts {
					mountPath, ok := volumeMountPathMap[esv1.StackConfigAdditionalSecretName(es.Name, secretMount.SecretName)]
					require.True(t, ok)
					require.Equal(t, secretMount.MountPath, mountPath)
				}
			}
		},
	}
}
