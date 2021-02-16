// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build es e2e

package es

import (
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
)

func TestReversalIllegalConfig(t *testing.T) {
	// 1 master node + 1 data node
	b := elasticsearch.NewBuilder("test-illegal-config").
		WithNoESTopology().
		WithESDataNodes(1, elasticsearch.DefaultResources).
		WithESMasterNodes(1, elasticsearch.DefaultResources)

	// then apply an illegal configuration change to the data node
	bogus := b.WithAdditionalConfig(map[string]map[string]interface{}{
		"data": {
			"this leads": "to a bootlooping instance",
		},
	})

	RunESMutationReversal(t, b, bogus)
}

func TestReversalRiskyMasterDownscale(t *testing.T) {
	// we create a non-ha cluster
	b := elasticsearch.NewBuilder("test-non-ha-downscale-reversal").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)
	// we then scale it down to 1 node, which for 6.x cluster in particular is a risky operation
	// after reversing we expect a cluster to re-form. There is some potential for data loss
	// in case the cluster indeed goes into split-brain.
	// TODO it might be necessary to accept some data loss for 6.x here
	down := b.WithNoESTopology().WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	RunESMutationReversal(t, b, down)
}

func TestReversalStatefulSetRename(t *testing.T) {
	b := elasticsearch.NewBuilder("test-sset-rename-reversal").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	copy := b.Elasticsearch.Spec.NodeSets[0]
	copy.Name = "other"
	renamed := b.WithNoESTopology().WithNodeSet(copy)

	RunESMutationReversal(t, b, renamed)
}

func TestRiskyMasterReconfiguration(t *testing.T) {
	b := elasticsearch.NewBuilder("test-sset-reconfig-reversal").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithNodeSet(esv1.NodeSet{
			Name:  "other-master",
			Count: 1,
			Config: &commonv1.Config{
				Data: map[string]interface{}{
					esv1.NodeMaster: true,
					esv1.NodeData:   true,
				},
			},
			PodTemplate: elasticsearch.ESPodTemplate(elasticsearch.DefaultResources),
		})

	// this currently breaks the cluster (something we might fix in the future at which point this just tests a temp downscale)
	noMasterMaster := b.WithNoESTopology().WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithNodeSet(esv1.NodeSet{
			Name:  "other-master",
			Count: 1,
			Config: &commonv1.Config{
				Data: map[string]interface{}{
					esv1.NodeMaster: false,
					esv1.NodeData:   true,
				},
			},
			PodTemplate: elasticsearch.ESPodTemplate(elasticsearch.DefaultResources),
		})

	RunESMutationReversal(t, b, noMasterMaster)
}

func RunESMutationReversal(t *testing.T, toCreate elasticsearch.Builder, mutateTo elasticsearch.Builder) {
	test.RunMutationReversal(t, []test.Builder{toCreate}, []test.Builder{mutateTo.WithMutatedFrom(&toCreate)})
}
