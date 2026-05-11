// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
)

const (
	// Type represents the APM Server type
	Type = "apm-server"
)

// GetIdentityLabels will return the common Elastic assigned labels for the APM Server.
func (as *ApmServer) GetIdentityLabels() map[string]string {
	return map[string]string{
		commonv1.TypeLabelName:    Type,
		"apm.k8s.elastic.co/name": as.Name,
	}
}
