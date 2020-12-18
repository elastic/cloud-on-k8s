// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package agent

import (
	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

// CalculateHealth returns health of the Agent based on association status, desired count and ready count.
func CalculateHealth(associations []v1.Association, ready, desired int32) agentv1alpha1.AgentHealth {
	for _, assoc := range associations {
		if assoc.AssociationConf().IsConfigured() {
			statusMap := assoc.AssociationStatusMap(assoc.AssociationType())
			if !statusMap.AllEstablished() {
				return agentv1alpha1.AgentRedHealth
			}
		}
	}

	switch {
	case ready == 0:
		return agentv1alpha1.AgentRedHealth
	case ready == desired:
		return agentv1alpha1.AgentGreenHealth
	case ready > 0:
		return agentv1alpha1.AgentYellowHealth
	default:
		return agentv1alpha1.AgentRedHealth
	}
}
