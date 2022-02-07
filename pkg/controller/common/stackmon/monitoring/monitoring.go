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

func AreAssocConfigured(resource HasMonitoring) bool {
	return IsMetricsAssocConfigured(resource) && IsLogsAssocConfigured(resource)
}

func IsMetricsAssocConfigured(resource HasMonitoring) bool {
	if !IsMetricsDefined(resource) {
		return false
	}
	refs := resource.GetMonitoringMetricsRefs()
	for _, ref := range refs {
		if !resource.MonitoringAssociation(ref).AssociationConf().IsConfigured() {
			return false
		}
	}
	return true
}

func IsLogsAssocConfigured(resource HasMonitoring) bool {
	if !IsLogsDefined(resource) {
		return false
	}
	refs := resource.GetMonitoringLogsRefs()
	for _, ref := range refs {
		if !resource.MonitoringAssociation(ref).AssociationConf().IsConfigured() {
			return false
		}
	}
	return true
}

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
