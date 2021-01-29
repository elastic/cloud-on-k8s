// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package autoscaler

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
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
	// ActualAutoscalingStatus is the current resources status as stored in the Elasticsearch resource.
	ActualAutoscalingStatus status.Status
	// RequiredCapacity contains the Elasticsearch Autoscaling API result.
	RequiredCapacity client.AutoscalingCapacityInfo
	// StatusBuilder is used to track any event that should be surfaced to the user.
	StatusBuilder *status.AutoscalingStatusBuilder
}
