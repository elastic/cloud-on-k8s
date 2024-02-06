// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

// GetIdentityLabels will return the common Elastic assigned labels for Logstash
func (logstash *Logstash) GetIdentityLabels() map[string]string {
	return map[string]string{
		commonv1.TypeLabelName:         "logstash",
		"logstash.k8s.elastic.co/name": logstash.Name,
	}
}

// GetPodIdentityLabels will return the common Elastic assigned labels for a Logstash Pod
func (logstash *Logstash) GetPodIdentityLabels() map[string]string {
	return maps.Merge(logstash.GetIdentityLabels(), map[string]string{
		"logstash.k8s.elastic.co/statefulset-name": Name(logstash.Name),
	})
}
