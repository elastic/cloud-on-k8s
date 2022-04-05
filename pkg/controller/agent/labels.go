// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
)

const (
	// TypeLabelValue represents the Agent type.
	TypeLabelValue = "agent"

	// NameLabelName used to represent an Agent in k8s resources
	NameLabelName = "agent.k8s.elastic.co/name"

	// NamespaceLabelName used to represent an Agent in k8s resources
	NamespaceLabelName = "agent.k8s.elastic.co/namespace"
)

// NewLabels returns the set of common labels for an Elastic Agent.
func NewLabels(agent agentv1alpha1.Agent) map[string]string {
	return map[string]string{
		common.TypeLabelName: TypeLabelValue,
		NameLabelName:        agent.Name,
	}
}
