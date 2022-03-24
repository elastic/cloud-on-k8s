// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

// CalculateHealth returns health of the Beat based on association status, desired count and ready count.
func CalculateHealth(associations []v1.Association, ready, desired int32) (beatv1beta1.BeatHealth, error) {
	for _, assoc := range associations {
		assocConf, err := assoc.AssociationConf()
		if err != nil {
			return "", err
		}
		if assocConf.IsConfigured() {
			statusMap := assoc.AssociationStatusMap(assoc.AssociationType())
			if !statusMap.AllEstablished() {
				return beatv1beta1.BeatRedHealth, nil
			}
		}
	}

	switch {
	case ready == 0:
		return beatv1beta1.BeatRedHealth, nil
	case ready == desired:
		return beatv1beta1.BeatGreenHealth, nil
	case ready > 0:
		return beatv1beta1.BeatYellowHealth, nil
	default:
		return beatv1beta1.BeatRedHealth, nil
	}
}
