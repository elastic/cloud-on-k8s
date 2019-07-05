// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodespec

import (
	appsv1 "k8s.io/api/apps/v1"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version"
	esvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

type NodeSpecResources struct {
	StatefulSet appsv1.StatefulSet
	Config      settings.CanonicalConfig
	// TLS certs
}

type NodeSpecResourcesList []NodeSpecResources

func (l NodeSpecResourcesList) StatefulSets() sset.StatefulSetList {
	ssets := make(sset.StatefulSetList, 0, len(l))
	for _, nodeSpec := range l {
		ssets = append(ssets, nodeSpec.StatefulSet)
	}
	return ssets
}

func ExpectedNodesResources(es v1alpha1.Elasticsearch, podTemplateBuilder version.PodTemplateSpecBuilder) (NodeSpecResourcesList, error) {
	nodesResources := make([]NodeSpecResources, 0, len(es.Spec.Nodes))

	for _, nodes := range es.Spec.Nodes {
		// build es config
		userCfg := commonv1alpha1.Config{}
		if nodes.Config != nil {
			userCfg = *nodes.Config
		}
		cfg, err := settings.NewMergedESConfig(es.Name, userCfg)
		if err != nil {
			return nil, err
		}

		// add default PVCs to the node spec
		// TODO: should this be done in another place?
		nodes.VolumeClaimTemplates = defaults.AppendDefaultPVCs(
			nodes.VolumeClaimTemplates, nodes.PodTemplate.Spec, esvolume.DefaultVolumeClaimTemplates...,
		)

		// build stateful set
		sset, err := sset.BuildStatefulSet(k8s.ExtractNamespacedName(&es), nodes, podTemplateBuilder)
		if err != nil {
			return nil, err
		}
		nodesResources = append(nodesResources, NodeSpecResources{
			StatefulSet: sset,
			Config:      cfg,
		})
	}
	return nodesResources, nil
}
