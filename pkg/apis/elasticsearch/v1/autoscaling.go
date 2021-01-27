// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/set"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
	"k8s.io/apimachinery/pkg/api/resource"
)

const ElasticsearchAutoscalingSpecAnnotationName = "elasticsearch.alpha.elastic.co/autoscaling-spec"

var errNodeRolesNotSet = errors.New("node.roles must be set")

// -- Elasticsearch Autoscaling API structures

// DeciderSettings allow the user to tweak autoscaling deciders.
// The map data structure complies with the <key,value> format expected by Elasticsearch.
// +kubebuilder:object:generate=false
type DeciderSettings map[string]string

// AutoscalingPolicy models the Elasticsearch autoscaling API.
// +kubebuilder:object:generate=false
type AutoscalingPolicy struct {
	// An autoscaling policy must target a unique set of roles.
	Roles []string `json:"roles,omitempty"`
	// Deciders allow the user to override default settings for autoscaling deciders.
	Deciders map[string]DeciderSettings `json:"deciders,omitempty"`
}

// -- Elastic Cloud on K8S specific structures

// AutoscalingSpec is the root object of the autoscaling specification in the Elasticsearch resource definition.
// +kubebuilder:object:generate=false
type AutoscalingSpec struct {
	AutoscalingPolicySpecs AutoscalingPolicySpecs `json:"policies"`
	// Elasticsearch is stored in the autoscaling spec for convenience. It should be removed once the autoscaling spec is
	// fully part of the Elasticsearch specification.
	Elasticsearch Elasticsearch `json:"-"`
}

// +kubebuilder:object:generate=false
type AutoscalingPolicySpecs []AutoscalingPolicySpec

// NamedAutoscalingPolicy models an autoscaling policy as expected by the Elasticsearch policy API.
// It is identified by a unique name provided by the user.
// +kubebuilder:object:generate=false
type NamedAutoscalingPolicy struct {
	// Name identifies the autoscaling policy in the autoscaling specification.
	Name string `json:"name,omitempty"`
	// AutoscalingPolicy is the autoscaling policy as expected by the Elasticsearch API.
	AutoscalingPolicy
}

// AutoscalingPolicySpec holds a named autoscaling policy and the associated resources limits (cpu, memory, storage).
// +kubebuilder:object:generate=false
type AutoscalingPolicySpec struct {
	NamedAutoscalingPolicy

	AutoscalingResources `json:"resources"`
}

// +kubebuilder:object:generate=false
// AutoscalingResources model the limits, submitted by the user, for the supported resources in an autoscaling policy.
// Only the node count range is mandatory. For other resources, a limit range is required only
// if the Elasticsearch autoscaling capacity API returns a requirement for a given resource.
// For example, the memory limit range is only required if the autoscaling API response contains a memory requirement.
// If there is no limit range for a resource, and if that resource is not mandatory, then the resources in the NodeSets
// managed by the autoscaling policy are left untouched.
type AutoscalingResources struct {
	CPU     *QuantityRange `json:"cpu,omitempty"`
	Memory  *QuantityRange `json:"memory,omitempty"`
	Storage *QuantityRange `json:"storage,omitempty"`

	// NodeCount is used to model the minimum and the maximum number of nodes over all the NodeSets managed by a same autoscaling policy.
	NodeCount CountRange `json:"nodeCount"`
}

// QuantityRange models a resource limit range for resources which can be expressed with resource.Quantity.
// +kubebuilder:object:generate=false
type QuantityRange struct {
	// Min represents the lower limit for the resources managed by the autoscaler.
	Min resource.Quantity `json:"min"`
	// Max represents the upper limit for the resources managed by the autoscaler.
	Max resource.Quantity `json:"max"`
	// RequestsToLimitsRatio allows to customize Kubernetes resource Limit based on the Request.
	RequestsToLimitsRatio *float64 `json:"requestsToLimitsRatio"`
}

// +kubebuilder:object:generate=false
type CountRange struct {
	// Min represents the minimum number of nodes in a tier.
	Min int32 `json:"min"`
	// Max represents the maximum number of nodes in a tier.
	Max int32 `json:"max"`
}

// GetAutoscalingSpecification unmarshal autoscaling specifications from an Elasticsearch resource.
func (es Elasticsearch) GetAutoscalingSpecification() (AutoscalingSpec, error) {
	autoscalingSpec := AutoscalingSpec{}
	if len(es.AutoscalingSpec()) == 0 {
		return autoscalingSpec, nil
	}
	err := json.Unmarshal([]byte(es.AutoscalingSpec()), &autoscalingSpec)
	autoscalingSpec.Elasticsearch = es
	return autoscalingSpec, err
}

// IsMemoryDefined returns true if the user specified memory limits.
func (aps AutoscalingPolicySpec) IsMemoryDefined() bool {
	return aps.Memory != nil
}

// IsCPUDefined returns true if the user specified cpu limits.
func (aps AutoscalingPolicySpec) IsCPUDefined() bool {
	return aps.CPU != nil
}

// IsStorageDefined returns true if the user specified storage limits.
func (aps AutoscalingPolicySpec) IsStorageDefined() bool {
	return aps.Storage != nil
}

// findByRoles returns the autoscaling specification associated with a set of roles or nil if not found.
func (as AutoscalingSpec) findByRoles(roles []string) *AutoscalingPolicySpec {
	for _, autoscalingPolicySpec := range as.AutoscalingPolicySpecs {
		if !rolesMatch(autoscalingPolicySpec.Roles, roles) {
			continue
		}
		return &autoscalingPolicySpec
	}
	return nil
}

// rolesMatch compares two set of roles and returns true if both sets contain the exact same roles.
func rolesMatch(roles1, roles2 []string) bool {
	if len(roles1) != len(roles2) {
		return false
	}
	rolesInRoles1 := set.Make(roles1...)
	for _, roleInRoles2 := range roles2 {
		if !rolesInRoles1.Has(roleInRoles2) {
			return false
		}
	}
	return true
}

// AutoscaledNodeSets holds the node sets managed by an autoscaling policy, indexed by the autoscaling policy name.
// +kubebuilder:object:generate=false
type AutoscaledNodeSets map[string]NodeSetList

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
func (as AutoscalingSpec) GetAutoscaledNodeSets() (AutoscaledNodeSets, *NodeSetConfigError) {
	namedTiersSet := make(AutoscaledNodeSets)
	for i, nodeSet := range as.Elasticsearch.Spec.NodeSets {
		resourcePolicy, err := as.GetAutoscalingSpecFor(nodeSet)
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

// GetMLNodesSettings computes the total number of ML nodes which can be deployed in the cluster and the maximum memory size
// of each node in the ML tier.
func (as AutoscalingSpec) GetMLNodesSettings() (nodes int32, maxMemory string) {
	var maxMemoryAsInt int64
	for _, autoscalingSpec := range as.AutoscalingPolicySpecs {
		if autoscalingSpec.IsMemoryDefined() &&
			stringsutil.StringInSlice(MLRole, autoscalingSpec.Roles) &&
			autoscalingSpec.Memory.Max.Value() > maxMemoryAsInt {
			maxMemoryAsInt = autoscalingSpec.Memory.Max.Value()
		}
		nodes += autoscalingSpec.NodeCount.Max
	}
	maxMemory = fmt.Sprintf("%db", maxMemoryAsInt)
	return nodes, maxMemory
}

// GetAutoscalingSpecFor retrieves the autoscaling spec associated to a NodeSet or nil if none.
func (as AutoscalingSpec) GetAutoscalingSpecFor(nodeSet NodeSet) (*AutoscalingPolicySpec, error) {
	v, err := version.Parse(as.Elasticsearch.Spec.Version)
	if err != nil {
		return nil, err
	}
	roles, err := getNodeSetRoles(*v, nodeSet)
	if err != nil {
		return nil, err
	}
	return as.findByRoles(roles), nil
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
