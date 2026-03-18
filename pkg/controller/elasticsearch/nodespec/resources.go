// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// Resources contain per-NodeSet resources to be created.
type Resources struct {
	NodeSet         string
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

func (l ResourcesList) StatefulSets() es_sset.StatefulSetList {
	ssetList := make(es_sset.StatefulSetList, 0, len(l))
	for _, resource := range l {
		ssetList = append(ssetList, resource.StatefulSet)
	}
	return ssetList
}

func (l ResourcesList) ExpectedNodeCount() int32 {
	return l.StatefulSets().ExpectedNodeCount()
}

// NodeSetConfig holds the pre-computed merged configuration for a single NodeSet.
type NodeSetConfig struct {
	NodeSetName string
	Config      settings.CanonicalConfig
}

// BuildExpectedResources builds the expected Kubernetes resources for all NodeSets.
// It uses pre-computed configs from nodeSetConfigs (computed early in reconciliation)
// to avoid duplicate config computation.
func BuildExpectedResources(
	ctx context.Context,
	client k8s.Client,
	es esv1.Elasticsearch,
	keystoreResources *keystore.Resources,
	existingStatefulSets es_sset.StatefulSetList,
	setDefaultSecurityContext bool,
	meta metadata.Metadata,
	nodeSetConfigs []NodeSetConfig,
	clientAuthenticationRequired bool,
	policyConfig PolicyConfig,
) (ResourcesList, error) {
	nodesResources := make(ResourcesList, 0, len(es.Spec.NodeSets))

	// we retrieve the current pods restart trigger annotation.
	actualPodsRestartTriggerAnnotationValue, err := es_sset.GetActualPodsRestartTriggerAnnotationForCluster(client, es)
	if err != nil {
		return nil, err
	}

	for i, nodeSpec := range es.Spec.NodeSets {
		// Get the pre-computed config for this NodeSet
		if i >= len(nodeSetConfigs) || nodeSetConfigs[i].NodeSetName != nodeSpec.Name {
			return nil, fmt.Errorf("nodeSetConfigs mismatch: expected config for %s at index %d", nodeSpec.Name, i)
		}
		cfg := nodeSetConfigs[i].Config

		statefulSet, err := BuildStatefulSet(ctx, client, es, nodeSpec, cfg, keystoreResources, existingStatefulSets, setDefaultSecurityContext, policyConfig, meta, actualPodsRestartTriggerAnnotationValue, clientAuthenticationRequired)
		if err != nil {
			return nil, err
		}
		headlessSvc := HeadlessService(&es, statefulSet.Name, meta)

		nodesResources = append(nodesResources, Resources{
			NodeSet:         nodeSpec.Name,
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
