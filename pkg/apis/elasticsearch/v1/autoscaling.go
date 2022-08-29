// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/set"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/stringsutil"
)

var (
	errNodeRolesNotSet = errors.New("node.roles must be set")
)

// AutoscaledNodeSets holds the node sets managed by an autoscaling policy, indexed by the autoscaling policy name.
type AutoscaledNodeSets map[string]NodeSetList

// Names returns the names of the node sets indexed by the autoscaling policy name.
func (n AutoscaledNodeSets) Names() map[string][]string {
	autoscalingPolicies := make(map[string][]string)
	for policy, nodeSetList := range n {
		autoscalingPolicies[policy] = nodeSetList.Names()
	}
	return autoscalingPolicies
}

// AutoscalingPolicies returns the list of autoscaling policies names from the named tiers.
func (n AutoscaledNodeSets) AutoscalingPolicies() set.StringSet {
	autoscalingPolicies := set.Make()
	for autoscalingPolicy := range n {
		autoscalingPolicies.Add(autoscalingPolicy)
	}
	return autoscalingPolicies
}

// +kubebuilder:object:generate=false
type NodeSetConfigError struct {
	error
	NodeSet
	Index int
}

// GetAutoscaledNodeSets retrieves the name of all the autoscaling policies in the Elasticsearch manifest and the associated NodeSets.
func (es *Elasticsearch) GetAutoscaledNodeSets(v version.Version, as v1alpha1.AutoscalingPolicySpecs) (AutoscaledNodeSets, *NodeSetConfigError) {
	namedTiersSet := make(AutoscaledNodeSets)
	if es == nil {
		return namedTiersSet, nil
	}
	for i, nodeSet := range es.Spec.NodeSets {
		resourcePolicy, err := nodeSet.GetAutoscalingSpecFor(v, as)
		if err != nil {
			return nil, &NodeSetConfigError{
				error:   err,
				NodeSet: nodeSet,
				Index:   i,
			}
		}
		if resourcePolicy == nil {
			// This nodeSet is not managed by an autoscaling policy
			continue
		}
		namedTiersSet[resourcePolicy.Name] = append(namedTiersSet[resourcePolicy.Name], *nodeSet.DeepCopy())
	}
	return namedTiersSet, nil
}

// GetAutoscalingSpecFor retrieves the autoscaling spec associated to a NodeSet or nil if none.
func (ns NodeSet) GetAutoscalingSpecFor(v version.Version, as v1alpha1.AutoscalingPolicySpecs) (*v1alpha1.AutoscalingPolicySpec, error) {
	roles, err := getNodeSetRoles(v, ns)
	if err != nil {
		return nil, err
	}
	return as.FindByRoles(roles), nil
}

// GetMLNodesSettings computes the total number of ML nodes which can be deployed in the cluster and the maximum memory size
// of each node in the ML tier.
func GetMLNodesSettings(as v1alpha1.AutoscalingPolicySpecs) (nodes int32, maxMemory string) {
	var maxMemoryAsInt int64
	for _, autoscalingSpec := range as {
		if !stringsutil.StringInSlice(string(MLRole), autoscalingSpec.Roles) {
			// not a node with the machine learning role
			continue
		}
		nodes += autoscalingSpec.NodeCountRange.Max
		if autoscalingSpec.IsMemoryDefined() && autoscalingSpec.MemoryRange.Max.Value() > maxMemoryAsInt {
			maxMemoryAsInt = autoscalingSpec.MemoryRange.Max.Value()
		}
	}
	maxMemory = fmt.Sprintf("%db", maxMemoryAsInt)
	return nodes, maxMemory
}

// getNodeSetRoles attempts to parse the roles specified in the configuration of a given nodeSet.
func getNodeSetRoles(v version.Version, nodeSet NodeSet) ([]string, error) {
	cfg := ElasticsearchSettings{}
	if err := UnpackConfig(nodeSet.Config, v, &cfg); err != nil {
		return nil, err
	}
	if cfg.Node == nil {
		return nil, errNodeRolesNotSet
	}
	return cfg.Node.Roles, nil
}

// -- Status

const ElasticsearchAutoscalingStatusAnnotationName = "elasticsearch.alpha.elastic.co/autoscaling-status"

// Deprecated: the autoscaling annotation has been deprecated in favor of the ElasticsearchAutoscaler custom resource.
func ElasticsearchAutoscalerStatusFrom(es Elasticsearch) (v1alpha1.ElasticsearchAutoscalerStatus, error) {
	status := v1alpha1.ElasticsearchAutoscalerStatus{}
	if es.Annotations == nil {
		return status, nil
	}
	serializedStatus, ok := es.Annotations[ElasticsearchAutoscalingStatusAnnotationName]
	if !ok {
		return status, nil
	}
	err := json.Unmarshal([]byte(serializedStatus), &status)
	return status, err
}

// Deprecated: the autoscaling annotation has been deprecated in favor of the ElasticsearchAutoscaler custom resource.
func UpdateAutoscalingStatus(
	es *Elasticsearch,
	elasticsearchAutoscalerStatus v1alpha1.ElasticsearchAutoscalerStatus,
) error {
	// Create the annotation
	if es.Annotations == nil {
		es.Annotations = make(map[string]string)
	}
	serializedStatus, err := json.Marshal(&elasticsearchAutoscalerStatus)
	if err != nil {
		return err
	}
	es.Annotations[ElasticsearchAutoscalingStatusAnnotationName] = string(serializedStatus)
	return nil
}
