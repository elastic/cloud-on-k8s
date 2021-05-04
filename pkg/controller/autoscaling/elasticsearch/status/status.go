// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package status

import (
	"encoding/json"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ElasticsearchAutoscalingStatusAnnotationName = "elasticsearch.alpha.elastic.co/autoscaling-status"

	EmptyResponse                 AutoscalingEventType = "EmptyResponse"
	HorizontalScalingLimitReached AutoscalingEventType = "HorizontalScalingLimitReached"
	MemoryRequired                AutoscalingEventType = "MemoryRequired"
	NoNodeSet                     AutoscalingEventType = "NoNodeSet"
	OverlappingPolicies           AutoscalingEventType = "OverlappingPolicies"
	StorageRequired               AutoscalingEventType = "StorageRequired"
	UnexpectedStorageCapacity     AutoscalingEventType = "UnexpectedStorageCapacity"
	VerticalScalingLimitReached   AutoscalingEventType = "VerticalScalingLimitReached"
)

type Status struct {
	// AutoscalingPolicyStatuses is used to expose state messages to user or external system
	AutoscalingPolicyStatuses []AutoscalingPolicyStatus `json:"policies"`
}

type AutoscalingPolicyStatus struct {
	// Name is the name of the autoscaling policy
	Name string `json:"name"`
	// NodeSetNodeCount holds the number of nodes for each nodeSet.
	NodeSetNodeCount resources.NodeSetNodeCountList `json:"nodeSets"`
	// ResourcesSpecification holds the resource values common to all the nodeSets managed by a same autoscaling policy.
	// Only the resources managed by the autoscaling controller are saved in the Status.
	ResourcesSpecification resources.NodeResources `json:"resources"`
	// PolicyStates may contain various messages regarding the current state of this autoscaling policy.
	PolicyStates []PolicyState `json:"state"`
	// LastModificationTime is the last time the resources have been updated, used by the cooldown algorithm.
	LastModificationTime metav1.Time `json:"lastModificationTime"`
}

func (s *Status) CurrentResourcesForPolicy(policyName string) (resources.NodeSetsResources, bool) {
	for _, policyStatus := range s.AutoscalingPolicyStatuses {
		if policyStatus.Name == policyName {
			return resources.NodeSetsResources{
				Name:             policyStatus.Name,
				NodeSetNodeCount: policyStatus.NodeSetNodeCount,
				NodeResources:    policyStatus.ResourcesSpecification,
			}, true
		}
	}
	return resources.NodeSetsResources{}, false
}

func (s *Status) LastModificationTime(policyName string) (metav1.Time, bool) {
	for _, policyState := range s.AutoscalingPolicyStatuses {
		if policyState.Name == policyName {
			return policyState.LastModificationTime, true
		}
	}
	return metav1.Time{}, false
}

type AutoscalingPolicyStatusBuilder struct {
	policyName           string
	nodeSetsResources    resources.NodeSetsResources
	lastModificationTime metav1.Time
	states               map[AutoscalingEventType]PolicyState
}

func NewAutoscalingPolicyStatusBuilder(name string) *AutoscalingPolicyStatusBuilder {
	return &AutoscalingPolicyStatusBuilder{
		policyName: name,
		states:     make(map[AutoscalingEventType]PolicyState),
	}
}

func (psb *AutoscalingPolicyStatusBuilder) Build() AutoscalingPolicyStatus {
	policyStates := make([]PolicyState, len(psb.states))
	i := 0
	for _, v := range psb.states {
		policyStates[i] = PolicyState{
			Type:     v.Type,
			Messages: v.Messages,
		}
		i++ //nolint:wastedassign
	}
	return AutoscalingPolicyStatus{
		Name:                   psb.policyName,
		NodeSetNodeCount:       psb.nodeSetsResources.NodeSetNodeCount,
		ResourcesSpecification: psb.nodeSetsResources.NodeResources,
		LastModificationTime:   psb.lastModificationTime,
		PolicyStates:           policyStates,
	}
}

// SetNodeSetsResources sets the compute resources associated to a tier.
func (psb *AutoscalingPolicyStatusBuilder) SetNodeSetsResources(nodeSetsResources resources.NodeSetsResources) *AutoscalingPolicyStatusBuilder {
	psb.nodeSetsResources = nodeSetsResources
	return psb
}

func (psb *AutoscalingPolicyStatusBuilder) SetLastModificationTime(lastModificationTime metav1.Time) *AutoscalingPolicyStatusBuilder {
	psb.lastModificationTime = lastModificationTime
	return psb
}

// RecordEvent records a new event (type + message) for the tier.
func (psb *AutoscalingPolicyStatusBuilder) RecordEvent(stateType AutoscalingEventType, message string) *AutoscalingPolicyStatusBuilder {
	if policyState, ok := psb.states[stateType]; ok {
		policyState.Messages = append(policyState.Messages, message)
		psb.states[stateType] = policyState
		return psb
	}
	psb.states[stateType] = PolicyState{
		Type:     stateType,
		Messages: []string{message},
	}
	return psb
}

type AutoscalingEventType string

type PolicyState struct {
	Type     AutoscalingEventType `json:"type"`
	Messages []string             `json:"messages"`
}

type AutoscalingStatusBuilder struct {
	policyStatusBuilder map[string]*AutoscalingPolicyStatusBuilder
}

// NewAutoscalingStatusBuilder creates a new autoscaling status builder.
func NewAutoscalingStatusBuilder() *AutoscalingStatusBuilder {
	return &AutoscalingStatusBuilder{
		policyStatusBuilder: make(map[string]*AutoscalingPolicyStatusBuilder),
	}
}

func (psb *AutoscalingStatusBuilder) ForPolicy(policyName string) *AutoscalingPolicyStatusBuilder {
	if value, ok := psb.policyStatusBuilder[policyName]; ok {
		return value
	}
	policyStatusBuilder := NewAutoscalingPolicyStatusBuilder(policyName)
	psb.policyStatusBuilder[policyName] = policyStatusBuilder
	return policyStatusBuilder
}

func (psb *AutoscalingStatusBuilder) Build() Status {
	policyStates := make([]AutoscalingPolicyStatus, len(psb.policyStatusBuilder))
	i := 0
	for _, policyStateBuilder := range psb.policyStatusBuilder {
		policyStates[i] = policyStateBuilder.Build()
		i++ //nolint:wastedassign
	}

	return Status{
		AutoscalingPolicyStatuses: policyStates,
	}
}

func From(es esv1.Elasticsearch) (Status, error) {
	status := Status{}
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

func UpdateAutoscalingStatus(
	es *esv1.Elasticsearch,
	statusBuilder *AutoscalingStatusBuilder,
	nextClusterResources resources.ClusterResources,
	currentAutoscalingStatus Status,
) error {
	// Update the timestamp on tiers resources
	now := metav1.Now()
	for _, nextNodeSetResources := range nextClusterResources {
		// Save the resources in the status
		statusBuilder.ForPolicy(nextNodeSetResources.Name).SetNodeSetsResources(nextNodeSetResources)

		// Restore the previous timestamp
		previousTimestamp, ok := currentAutoscalingStatus.LastModificationTime(nextNodeSetResources.Name)
		if ok {
			statusBuilder.ForPolicy(nextNodeSetResources.Name).SetLastModificationTime(previousTimestamp)
		}

		currentNodeSetResources, ok := currentAutoscalingStatus.CurrentResourcesForPolicy(nextNodeSetResources.Name)
		if !ok || !currentNodeSetResources.SameResources(nextNodeSetResources) {
			statusBuilder.ForPolicy(nextNodeSetResources.Name).SetLastModificationTime(now)
		}
	}

	// Create the annotation
	if es.Annotations == nil {
		es.Annotations = make(map[string]string)
	}
	status := statusBuilder.Build()
	serializedStatus, err := json.Marshal(&status)
	if err != nil {
		return err
	}
	es.Annotations[ElasticsearchAutoscalingStatusAnnotationName] = string(serializedStatus)
	return nil
}
