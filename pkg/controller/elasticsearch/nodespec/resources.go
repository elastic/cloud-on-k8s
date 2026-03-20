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

// ResolvedConfig holds all pre-computed configuration needed for reconciliation.
// Computing this early allows us to detect clientAuthenticationRequired before creating the ES client,
// and avoids duplicate config computation in BuildExpectedResources.
type ResolvedConfig struct {
	// NodeSetConfigs contains the merged configuration for each NodeSet, keyed by NodeSet name.
	NodeSetConfigs map[string]settings.CanonicalConfig

	// ClientAuthenticationRequired indicates whether client certificate authentication is required
	// based on the merged configuration.
	ClientAuthenticationRequired bool

	// PolicyConfig contains StackConfigPolicy settings.
	PolicyConfig PolicyConfig

	// ClientAuthenticationOverrideWarning is set when spec.http.tls.client.authentication is enabled
	// but StackConfigPolicy overrides xpack.security.http.ssl.client_authentication to a non-required value.
	ClientAuthenticationOverrideWarning string
}

// BuildExpectedResources builds the expected Kubernetes resources for all NodeSets.
// It uses pre-computed configs from ResolvedConfig (computed early in reconciliation)
// to avoid duplicate config computation.
func BuildExpectedResources(
	ctx context.Context,
	client k8s.Client,
	es esv1.Elasticsearch,
	keystoreResources *keystore.Resources,
	existingStatefulSets es_sset.StatefulSetList,
	setDefaultSecurityContext bool,
	meta metadata.Metadata,
	resolvedConfig ResolvedConfig,
) (ResourcesList, error) {
	nodesResources := make(ResourcesList, 0, len(es.Spec.NodeSets))

	// we retrieve the current pods restart trigger annotation.
	actualPodsRestartTriggerAnnotationValue, err := es_sset.GetActualPodsRestartTriggerAnnotationForCluster(client, es)
	if err != nil {
		return nil, err
	}

	for _, nodeSpec := range es.Spec.NodeSets {
		cfg, ok := resolvedConfig.NodeSetConfigs[nodeSpec.Name]
		if !ok {
			return nil, fmt.Errorf("no pre-computed config for NodeSet %s", nodeSpec.Name)
		}

		statefulSet, err := BuildStatefulSet(ctx, client, es, nodeSpec, cfg, keystoreResources, existingStatefulSets, setDefaultSecurityContext, resolvedConfig.PolicyConfig, meta, actualPodsRestartTriggerAnnotationValue, resolvedConfig.ClientAuthenticationRequired)
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
