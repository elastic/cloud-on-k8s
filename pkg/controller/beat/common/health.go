// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

// CalculateHealth returns health of the Beat based on association status, desired count and ready count.
func CalculateHealth(associations []v1.Association, ready, desired int32) beatv1beta1.BeatHealth {
	for _, assoc := range associations {
		if assoc.AssociationConf().IsConfigured() {
			statusMap := assoc.AssociationStatusMap(assoc.AssociationType())
			if !statusMap.AllEstablished() {
				return beatv1beta1.BeatRedHealth
			}
		}
	}

	switch {
	case ready == 0:
		return beatv1beta1.BeatRedHealth
	case ready == desired:
		return beatv1beta1.BeatGreenHealth
	case ready > 0:
		return beatv1beta1.BeatYellowHealth
	default:
		return beatv1beta1.BeatRedHealth
	}
}
