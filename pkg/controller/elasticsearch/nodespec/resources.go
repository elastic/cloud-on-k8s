// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"context"
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/stackconfigpolicy"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// Resources contain per-NodeSet resources to be created.
type Resources struct {
	NodeSet         string
	StatefulSet     appsv1.StatefulSet
	HeadlessService corev1.Service
	Config          settings.CanonicalConfig
}

type ResourcesList []Resources

type StackConfigPolicySecretHash struct {
	ElasticsearchConfigHash string
	SecretMountsHash        string
}

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

func (l ResourcesList) ExpectedNodeCount() int32 {
	return l.StatefulSets().ExpectedNodeCount()
}

func BuildExpectedResources(
	ctx context.Context,
	client k8s.Client,
	es esv1.Elasticsearch,
	keystoreResources *keystore.Resources,
	existingStatefulSets sset.StatefulSetList,
	ipFamily corev1.IPFamily,
	setDefaultSecurityContext bool,
	stackConfigPolicyConfigSecret corev1.Secret,
) (ResourcesList, error) {
	nodesResources := make(ResourcesList, 0, len(es.Spec.NodeSets))

	ver, err := version.Parse(es.Spec.Version)
	if err != nil {
		return nil, err
	}

	stackConfigPolicySecretHash := StackConfigPolicySecretHash{
		ElasticsearchConfigHash: stackConfigPolicyConfigSecret.Annotations[stackconfigpolicy.ElasticsearchConfigHashAnnotation],
		SecretMountsHash:        stackConfigPolicyConfigSecret.Annotations[stackconfigpolicy.SecretMountsHashAnnotation],
	}
	// Parse Elasticsearch config from the stack config policy secret.
	var esConfigFromStackConfigPolicy map[string]interface{}
	if string(stackConfigPolicyConfigSecret.Data["elasticsearch.yml"]) != "" {
		err = json.Unmarshal(stackConfigPolicyConfigSecret.Data["elasticsearch.yml"], &esConfigFromStackConfigPolicy)
		if err != nil {
			return nil, err
		}
	}

	var additionalSecretMounts []policyv1alpha1.SecretMount
	if string(stackConfigPolicyConfigSecret.Data["secretMounts.json"]) != "" {
		err = json.Unmarshal(stackConfigPolicyConfigSecret.Data["secretMounts.json"], &additionalSecretMounts)
		if err != nil {
			return nil, err
		}
	}

	// Create a New Config from the parsed config
	esConfigStackConfigPolicy := commonv1.NewConfig(esConfigFromStackConfigPolicy)

	for _, nodeSpec := range es.Spec.NodeSets {
		// build es config
		userCfg := commonv1.Config{}
		if nodeSpec.Config != nil {
			userCfg = *nodeSpec.Config
		}
		cfg, err := settings.NewMergedESConfig(es.Name, ver, ipFamily, es.Spec.HTTP, userCfg, esConfigStackConfigPolicy)
		if err != nil {
			return nil, err
		}

		// build stateful set and associated headless service
		statefulSet, err := BuildStatefulSet(ctx, client, es, nodeSpec, cfg, keystoreResources, existingStatefulSets, setDefaultSecurityContext, additionalSecretMounts, stackConfigPolicySecretHash)
		if err != nil {
			return nil, err
		}
		headlessSvc := HeadlessService(&es, statefulSet.Name)

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
