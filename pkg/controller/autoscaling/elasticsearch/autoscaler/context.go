// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoscaler

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/autoscaler/recommender"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/status"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/go-logr/logr"
)

// Context contains the required objects used by the autoscaler functions.
type Context struct {
	Log logr.Logger
	// AutoscalingSpec is the autoscaling specification as provided by the user.
	AutoscalingSpec esv1.AutoscalingPolicySpec
	// NodeSets is the list of the NodeSets managed by the autoscaling specification.
	NodeSets esv1.NodeSetList
	// CurrentAutoscalingStatus is the current resources status as stored in the Elasticsearch resource.
	CurrentAutoscalingStatus status.Status
	// AutoscalingPolicyResult contains the Elasticsearch Autoscaling API result.
	AutoscalingPolicyResult client.AutoscalingPolicyResult
	// StatusBuilder is used to track any event that should be surfaced to the user.
	StatusBuilder *status.AutoscalingStatusBuilder
	// Recommender are specialized services to compute required resources.
	Recommenders []recommender.Recommender
}

func NewContext(
	log logr.Logger,
	autoscalingSpec esv1.AutoscalingPolicySpec,
	nodeSets esv1.NodeSetList,
	currentAutoscalingStatus status.Status,
	autoscalingPolicyResult client.AutoscalingPolicyResult,
	statusBuilder *status.AutoscalingStatusBuilder,
) (*Context, error) {
	storageRecommender, err := recommender.NewStorageRecommender(
		log,
		statusBuilder,
		autoscalingSpec,
		autoscalingPolicyResult,
		currentAutoscalingStatus,
	)
	if err != nil {
		return nil, err
	}

	memoryRecommender, err := recommender.NewMemoryRecommender(
		log,
		statusBuilder,
		autoscalingSpec,
		autoscalingPolicyResult,
		currentAutoscalingStatus,
	)
	if err != nil {
		return nil, err
	}

	return &Context{
		Log:                      log,
		AutoscalingSpec:          autoscalingSpec,
		NodeSets:                 nodeSets,
		AutoscalingPolicyResult:  autoscalingPolicyResult,
		CurrentAutoscalingStatus: currentAutoscalingStatus,
		StatusBuilder:            statusBuilder,
		Recommenders:             []recommender.Recommender{storageRecommender, memoryRecommender},
	}, nil
}
