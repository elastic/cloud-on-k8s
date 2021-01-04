// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package agent

import (
	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
)

const (
	// Type represents the Agent type.
	TypeLabelValue = "agent"

	NameLabelName = "agent.k8s.elastic.co/name"
)

func NewLabels(agent agentv1alpha1.Agent) map[string]string {
	return map[string]string{
		common.TypeLabelName: TypeLabelValue,
		NameLabelName:        agent.Name,
	}
}
