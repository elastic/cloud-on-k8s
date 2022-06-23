// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
)

// TestCoordinatingNodes tests a cluster with coordinating nodes.
func TestCoordinatingNodes(t *testing.T) {
	b := elasticsearch.NewBuilder("test-es-coord").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithESCoordinatingNodes(1, elasticsearch.DefaultResources)

	test.Sequence(nil, test.EmptySteps, b).RunSequential(t)
}

// TestResourcesRequirements tests a cluster with resources (cpu/memory/storage) requirements.
func TestResourcesRequirements(t *testing.T) {
	b := elasticsearch.NewBuilder("test-es-resreq").
		WithNodeSet(esv1.NodeSet{
			Name:  "masterdata",
			Count: int32(1),
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					SecurityContext: test.DefaultSecurityContext(),
					Containers: []corev1.Container{
						{
							Name: esv1.ElasticsearchContainerName,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("4Gi"),
									corev1.ResourceCPU:    resource.MustParse("1000m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("4Gi"),
									corev1.ResourceCPU:    resource.MustParse("1600m"),
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				newPVC("2Gi", elasticsearch.DefaultStorageClass),
			},
		})

	test.Sequence(nil, test.EmptySteps, b).RunSequential(t)
}
