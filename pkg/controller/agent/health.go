// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

// CalculateHealth returns health of the Agent based on association status, desired count and ready count.
func CalculateHealth(associations []v1.Association, ready, desired int32) (agentv1alpha1.AgentHealth, error) {
	for _, assoc := range associations {
		assocConf, err := assoc.AssociationConf()
		if err != nil {
			return "", err
		}
		if assocConf.IsConfigured() {
			statusMap := assoc.AssociationStatusMap(assoc.AssociationType())
			if !statusMap.AllEstablished() {
				return agentv1alpha1.AgentRedHealth, nil
			}
		}
	}

	switch {
	case ready == 0:
		return agentv1alpha1.AgentRedHealth, nil
	case ready == desired:
		return agentv1alpha1.AgentGreenHealth, nil
	case ready > 0:
		return agentv1alpha1.AgentYellowHealth, nil
	default:
		return agentv1alpha1.AgentRedHealth, nil
	}
}
