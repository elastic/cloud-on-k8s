// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HasMonitoring is the interface implemented by an Elastic Stack application that supports Stack Monitoring ()
type HasMonitoring interface {
	metav1.Object
	client.Object
	GetMonitoringMetricsRefs() []commonv1.ObjectSelector
	GetMonitoringLogsRefs() []commonv1.ObjectSelector
	MonitoringAssociation(ref commonv1.ObjectSelector) commonv1.Association
}

func IsMonitoringDefined(resource HasMonitoring) bool {
	return IsMonitoringMetricsDefined(resource) || IsMonitoringLogsDefined(resource)
}

func IsMonitoringMetricsDefined(resource HasMonitoring) bool {
	return areEsRefsDefined(resource.GetMonitoringMetricsRefs())
}

func IsMonitoringLogsDefined(resource HasMonitoring) bool {
	return areEsRefsDefined(resource.GetMonitoringLogsRefs())
}

func areEsRefsDefined(esRefs []commonv1.ObjectSelector) bool {
	for _, ref := range esRefs {
		if !ref.IsDefined() {
			return false
		}
	}
	return len(esRefs) > 0
}

func GetMonitoringMetricsAssociation(resource HasMonitoring) []commonv1.Association {
	associations := make([]commonv1.Association, 0)
	for _, ref := range resource.GetMonitoringMetricsRefs() {
		if ref.IsDefined() {
			associations = append(associations, resource.MonitoringAssociation(ref))
		}
	}
	return associations
}

func GetMonitoringLogsAssociation(resource HasMonitoring) []commonv1.Association {
	associations := make([]commonv1.Association, 0)
	for _, ref := range resource.GetMonitoringLogsRefs() {
		if ref.IsDefined() {
			associations = append(associations, resource.MonitoringAssociation(ref))
		}
	}
	return associations
}
