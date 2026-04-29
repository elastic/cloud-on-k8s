// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
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
			Resources: commonv1.Resources{
				Requests: commonv1.ResourceAllocations{
					Memory: ptr.To(resource.MustParse("4Gi")),
					CPU:    ptr.To(resource.MustParse("1000m")),
				},
				Limits: commonv1.ResourceAllocations{
					Memory: ptr.To(resource.MustParse("4Gi")),
					CPU:    ptr.To(resource.MustParse("1600m")),
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				newPVC("2Gi", test.DefaultStorageClass),
			},
		})

	test.Sequence(nil, test.EmptySteps, b).RunSequential(t)
}
