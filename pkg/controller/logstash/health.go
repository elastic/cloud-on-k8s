// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	v1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
)

// CalculateHealth returns health of Logstash based on association status, desired count and ready count.
func CalculateHealth(associations []v1.Association, ready, desired int32) (lsv1alpha1.LogstashHealth, error) {
	for _, assoc := range associations {
		assocConf, err := assoc.AssociationConf()
		if err != nil {
			return "", err
		}
		if assocConf.IsConfigured() {
			statusMap := assoc.AssociationStatusMap(assoc.AssociationType())
			if !statusMap.AllEstablished() || len(statusMap) == 0 {
				return lsv1alpha1.LogstashRedHealth, nil
			}
		}
	}

	switch {
	case ready == 0:
		return lsv1alpha1.LogstashRedHealth, nil
	case ready == desired:
		return lsv1alpha1.LogstashGreenHealth, nil
	case ready > 0:
		return lsv1alpha1.LogstashYellowHealth, nil
	default:
		return lsv1alpha1.LogstashRedHealth, nil
	}
}
