// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// Resources contain per-NodeSet resources to be created.
type Resources struct {
	StatefulSet     appsv1.StatefulSet
	HeadlessService corev1.Service
	Config          settings.CanonicalConfig
}

type ResourcesList []Resources

func (l ResourcesList) ForStatefulSet(name string) (Resources, error) {
	for _, resource := range l {
		if resource.StatefulSet.Name == name {
			return resource, nil
		}
	}
	return Resources{}, fmt.Errorf("no expected resources for StatefulSet %s", name)
}

func (l ResourcesList) StatefulSets() sset.StatefulSetList {
	ssetList := make(sset.StatefulSetList, 0, len(l))
	for _, resource := range l {
		ssetList = append(ssetList, resource.StatefulSet)
	}
	return ssetList
}

func BuildExpectedResources(
	client k8s.Client,
	es esv1.Elasticsearch,
	keystoreResources *keystore.Resources,
	existingStatefulSets sset.StatefulSetList,
	ipFamily corev1.IPFamily,
	setDefaultSecurityContext bool,
) (ResourcesList, error) {
	nodesResources := make(ResourcesList, 0, len(es.Spec.NodeSets))

	ver, err := version.Parse(es.Spec.Version)
	if err != nil {
		return nil, err
	}

	for _, nodeSpec := range es.Spec.NodeSets {
		// build es config
		userCfg := commonv1.Config{}
		if nodeSpec.Config != nil {
			userCfg = *nodeSpec.Config
		}
		cfg, err := settings.NewMergedESConfig(es.Name, ver, ipFamily, es.Spec.HTTP, userCfg)
		if err != nil {
			return nil, err
		}

		// build stateful set and associated headless service
		statefulSet, err := BuildStatefulSet(client, es, nodeSpec, cfg, keystoreResources, existingStatefulSets, setDefaultSecurityContext)
		if err != nil {
			return nil, err
		}
		headlessSvc := HeadlessService(&es, statefulSet.Name)

		nodesResources = append(nodesResources, Resources{
			StatefulSet:     statefulSet,
			HeadlessService: headlessSvc,
			Config:          cfg,
		})
	}

	return nodesResources, nil
}

// MasterNodesNames returns the names of the master nodes for this ResourcesList.
func (l ResourcesList) MasterNodesNames() []string {
	var masters []string
	for _, s := range l.StatefulSets() {
		if label.IsMasterNodeSet(s) {
			for i := int32(0); i < sset.GetReplicas(s); i++ {
				masters = append(masters, sset.PodName(s.Name, i))
			}
		}
	}

	return masters
}
