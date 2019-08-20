// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"testing"

	common "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/elasticsearch"
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

	test.RunMutationReversal(t, []test.Builder{b}, []test.Builder{bogus})
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

	test.RunMutationReversal(t, []test.Builder{b}, []test.Builder{down})
}

func TestReversalStatefulSetRename(t *testing.T) {
	b := elasticsearch.NewBuilder("test-sset-rename-reversal").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	copy := b.Elasticsearch.Spec.Nodes[0]
	copy.Name = "other"
	renamed := b.WithNoESTopology().WithNodeSpec(copy)

	test.RunMutationReversal(t, []test.Builder{b}, []test.Builder{renamed})
}

func TestRiskyMasterReconfiguration(t *testing.T) {
	b := elasticsearch.NewBuilder("test-sset-reconfig-reversal").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithNodeSpec(v1alpha1.NodeSpec{
			Name:      "other-master",
			NodeCount: 1,
			Config: &common.Config{
				Data: map[string]interface{}{
					v1alpha1.NodeMaster: true,
					v1alpha1.NodeData:   true,
				},
			},
			PodTemplate: elasticsearch.ESPodTemplate(elasticsearch.DefaultResources),
		})

	// this currently breaks the cluster (something we might fix in the future at which point this just tests a temp downscale)
	noMasterMaster := b.WithNoESTopology().WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithNodeSpec(v1alpha1.NodeSpec{
			Name:      "other-master",
			NodeCount: 1,
			Config: &common.Config{
				Data: map[string]interface{}{
					v1alpha1.NodeMaster: false,
					v1alpha1.NodeData:   true,
				},
			},
			PodTemplate: elasticsearch.ESPodTemplate(elasticsearch.DefaultResources),
		})

	test.RunMutationReversal(t, []test.Builder{b}, []test.Builder{noMasterMaster})
}
