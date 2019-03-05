// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import "github.com/elastic/k8s-operators/operators/pkg/controller/common"

const (
	// ApmServerNameLabelName used to represent a Kibana in k8s resources
	ApmServerNameLabelName = "apm.k8s.elastic.co/name"
	// Type represents the apm server type
	Type = "apm-server"
)

// NewLabels constructs a new set of labels for an ApmServer pod
func NewLabels(apmServerName string) map[string]string {
	return map[string]string{
		ApmServerNameLabelName: apmServerName,
		common.TypeLabelName:   Type,
	}
}
