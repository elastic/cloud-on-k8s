// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package monitoring

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

// HasMonitoring is the interface implemented by an Elastic Stack application that supports Stack Monitoring
type HasMonitoring interface {
	client.Object
	GetMonitoringMetricsRefs() []commonv1.ObjectSelector
	GetMonitoringLogsRefs() []commonv1.ObjectSelector
	MonitoringAssociation(ref commonv1.ObjectSelector) commonv1.Association
}

// IsReconcilable return true if a resource has at least one association defined in its specification
// and all defined associations are configured.
func IsReconcilable(resource HasMonitoring) (bool, error) {
	if !IsDefined(resource) {
		return false, nil
	}
	allRefs := append(resource.GetMonitoringMetricsRefs(), resource.GetMonitoringLogsRefs()...)
	for _, ref := range allRefs {
		assocConf, err := resource.MonitoringAssociation(ref).AssociationConf()
		if err != nil {
			return false, err
		}
		if !assocConf.IsConfigured() {
			return false, nil
		}
	}
	return true, nil
}

// IsDefined return true if a resource has at least one association for Stack Monitoring defined in its specification
// (can be one for logs or one for metrics).
func IsDefined(resource HasMonitoring) bool {
	return IsMetricsDefined(resource) || IsLogsDefined(resource)
}

func IsMetricsDefined(resource HasMonitoring) bool {
	return AreEsRefsDefined(resource.GetMonitoringMetricsRefs())
}

func IsLogsDefined(resource HasMonitoring) bool {
	return AreEsRefsDefined(resource.GetMonitoringLogsRefs())
}

func AreEsRefsDefined(esRefs []commonv1.ObjectSelector) bool {
	for _, ref := range esRefs {
		if !ref.IsDefined() {
			return false
		}
	}
	return len(esRefs) > 0
}

func GetMetricsAssociation(resource HasMonitoring) []commonv1.Association {
	associations := make([]commonv1.Association, 0)
	for _, ref := range resource.GetMonitoringMetricsRefs() {
		if ref.IsDefined() {
			associations = append(associations, resource.MonitoringAssociation(ref))
		}
	}
	return associations
}

func GetLogsAssociation(resource HasMonitoring) []commonv1.Association {
	associations := make([]commonv1.Association, 0)
	for _, ref := range resource.GetMonitoringLogsRefs() {
		if ref.IsDefined() {
			associations = append(associations, resource.MonitoringAssociation(ref))
		}
	}
	return associations
}
