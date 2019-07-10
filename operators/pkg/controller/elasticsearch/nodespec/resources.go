// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodespec

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

// Resources contain per-NodeSpec resources to be created.
type Resources struct {
	StatefulSet     appsv1.StatefulSet
	HeadlessService corev1.Service
	Config          settings.CanonicalConfig
	// TLS certs
}

type ResourcesList []Resources

func (l ResourcesList) StatefulSets() sset.StatefulSetList {
	ssetList := make(sset.StatefulSetList, 0, len(l))
	for _, nodeSpec := range l {
		ssetList = append(ssetList, nodeSpec.StatefulSet)
	}
	return ssetList
}

func BuildExpectedResources(es v1alpha1.Elasticsearch, podTemplateBuilder version.PodTemplateSpecBuilder) (ResourcesList, error) {
	nodesResources := make(ResourcesList, 0, len(es.Spec.Nodes))

	for _, nodeSpec := range es.Spec.Nodes {
		// build es config
		userCfg := commonv1alpha1.Config{}
		if nodeSpec.Config != nil {
			userCfg = *nodeSpec.Config
		}
		cfg, err := settings.NewMergedESConfig(es.Name, userCfg)
		if err != nil {
			return nil, err
		}

		// build stateful set and associated headless service
		statefulSet, err := sset.BuildStatefulSet(k8s.ExtractNamespacedName(&es), nodeSpec, cfg, podTemplateBuilder)
		if err != nil {
			return nil, err
		}
		headlessSvc := sset.HeadlessService(k8s.ExtractNamespacedName(&es), statefulSet.Name)

		nodesResources = append(nodesResources, Resources{
			StatefulSet:     statefulSet,
			HeadlessService: headlessSvc,
			Config:          cfg,
		})
	}
	return nodesResources, nil
}
