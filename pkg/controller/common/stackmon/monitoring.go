// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HasMonitoring is the interface implemented by an Elastic Stack application that supports Stack Monitoring ()
type HasMonitoring interface {
	metav1.Object
	GetMonitoringMetricsAssociation() []commonv1.Association
	GetMonitoringLogsAssociation() []commonv1.Association
}
