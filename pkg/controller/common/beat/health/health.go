// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package health

import (
	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

type BeatHealth string

const (
	// BeatRedHealth means that the health is neither yellow nor green
	BeatRedHealth BeatHealth = "red"

	// BeatYellowHealth means that:
	// 1) at least one Pod is Ready, and
	// 1) association is not configured or configured and established
	BeatYellowHealth BeatHealth = "yellow"

	// BeatGreenHealth means that:
	// 1) all Pods are Ready, and
	// 2) association is not configured or configured and established
	BeatGreenHealth BeatHealth = "green"
)

// CalculateHealth returns health of the Beat calculated based on association status, desired count and ready count.
func CalculateHealth(associated v1.Associated, ready, desired int32) BeatHealth {
	if associated.AssociationConf().IsConfigured() && associated.AssociationStatus() != v1.AssociationEstablished {
		return BeatRedHealth
	}

	switch {
	case ready == desired:
		return BeatGreenHealth
	case ready > 0:
		return BeatYellowHealth
	default:
		return BeatRedHealth
	}
}
