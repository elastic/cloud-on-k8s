// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"context"
	"fmt"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
)

type AutoscalingClient interface {
	// DeleteAutoscalingPolicies deletes all the autoscaling policies in a cluster.
	DeleteAutoscalingPolicies(ctx context.Context) error
	// CreateAutoscalingPolicy creates a new autoscaling policy.
	CreateAutoscalingPolicy(ctx context.Context, policyName string, autoscalingPolicy esv1.AutoscalingPolicy) error
	// GetAutoscalingCapacity returns the capacity for the autoscaling policies declared in a cluster.
	GetAutoscalingCapacity(ctx context.Context) (AutoscalingCapacityResult, error)
	// UpdateMLNodesSettings helps to manage machine learning settings required by the ML decider to work correctly.
	UpdateMLNodesSettings(ctx context.Context, maxLazyMLNodes int32, maxMemory string) error
}

// MachineLearningSettings is used to build a request to update ML related settings for autoscaling.
type MachineLearningSettings struct {
	PersistentSettings *MachineLearningSettingsGroup `json:"persistent,omitempty"`
}

// MachineLearningSettingsGroup is a group of persistent settings.
type MachineLearningSettingsGroup struct {
	MaxMemory                   string `json:"xpack.ml.max_ml_node_size"`
	MaxLazyMLNodes              int32  `json:"xpack.ml.max_lazy_ml_nodes"`
	UseAutoMachineMemoryPercent bool   `json:"xpack.ml.use_auto_machine_memory_percent"`
}

func (c *clientV7) CreateAutoscalingPolicy(ctx context.Context, policyName string, autoscalingPolicy esv1.AutoscalingPolicy) error {
	path := fmt.Sprintf("/_autoscaling/policy/%s", policyName)
	return c.put(ctx, path, autoscalingPolicy, nil)
}

func (c *clientV7) DeleteAutoscalingPolicies(ctx context.Context) error {
	return c.delete(ctx, "/_autoscaling/policy/*")
}

func (c *clientV7) UpdateMLNodesSettings(ctx context.Context, maxLazyMLNodes int32, maxMemory string) error {
	return c.put(
		ctx,
		"/_cluster/settings",
		&MachineLearningSettings{
			&MachineLearningSettingsGroup{
				MaxLazyMLNodes:              maxLazyMLNodes,
				MaxMemory:                   maxMemory,
				UseAutoMachineMemoryPercent: true,
			}}, nil)
}

// AutoscalingCapacityResult models autoscaling capacity decisions. It maps each autoscaling policy to its result.
type AutoscalingCapacityResult struct {
	Policies map[string]AutoscalingPolicyResult `json:"policies"`
}

type AutoscalingPolicyResult struct {
	RequiredCapacity AutoscalingCapacityInfo `json:"required_capacity"`
	CurrentCapacity  AutoscalingCapacityInfo `json:"current_capacity"`
	CurrentNodes     []AutoscalingNodeInfo   `json:"current_nodes"`
}

// AutoscalingCapacityInfo models capacity information as received by the autoscaling Elasticsearch API.
type AutoscalingCapacityInfo struct {
	Node  AutoscalingResources `yaml:"node" json:"node,omitempty"`
	Total AutoscalingResources `yaml:"total" json:"total,omitempty"`
}

type AutoscalingNodeInfo struct {
	Name string `json:"name"`
}

// IsEmpty returns true if all the resource values are empty (no values in the API response).
// 0 is considered as a value since deciders are allowed to return 0 to fully scale down a tier.
func (ac AutoscalingCapacityInfo) IsEmpty() bool {
	return ac.Node.IsEmpty() && ac.Total.IsEmpty()
}

// AutoscalingCapacity models a capacity value as received by Elasticsearch.
type AutoscalingCapacity int64

// Value return the int64 value returned by Elasticsearch. It returns 0 if no value has been set by Elasticsearch.
func (ac *AutoscalingCapacity) Value() int64 {
	if ac == nil {
		return 0
	}
	return int64(*ac)
}

// IsEmpty returns true if the value is nil.
func (ac *AutoscalingCapacity) IsEmpty() bool {
	return ac == nil
}

// IsZero returns true if the value is greater than 0.
func (ac *AutoscalingCapacity) IsZero() bool {
	return ac.Value() == 0
}

type AutoscalingResources struct {
	Storage *AutoscalingCapacity `yaml:"storage" json:"storage,omitempty"`
	Memory  *AutoscalingCapacity `yaml:"memory" json:"memory,omitempty"`
}

// IsEmpty returns true if all the resource values are empty (no values, 0 being considered as a value).
// Expressed in a different way, it returns true if no resource as been returned in the autoscaling API response.
func (ar *AutoscalingResources) IsEmpty() bool {
	if ar == nil {
		return true
	}
	return ar.Memory.IsEmpty() && ar.Storage.IsEmpty()
}

// IsZero returns true if all the resource values are evaluated to 0.
// It also returns true if no value has been set, to check if the value exists in the API response see IsEmpty().
func (ar *AutoscalingResources) IsZero() bool {
	if ar == nil {
		return true
	}
	return ar.Memory.IsZero() && ar.Storage.IsZero()
}

func (c *clientV7) GetAutoscalingCapacity(ctx context.Context) (AutoscalingCapacityResult, error) {
	var response AutoscalingCapacityResult
	err := c.get(ctx, "/_autoscaling/capacity", &response)
	return response, err
}
