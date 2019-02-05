// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import "github.com/elastic/k8s-operators/operators/pkg/controller/common"

const (
	// KibanaNameLabelName used to represent a Kibana in k8s resources
	KibanaNameLabelName = "kibana.k8s.elastic.co/name"
	// Type represents the elasticsearch type
	Type = "elasticsearch"
)

// NewLabels constructs a new set of labels for a Kibana pod
func NewLabels(kibanaName string) map[string]string {
	return map[string]string{
		KibanaNameLabelName:  kibanaName,
		common.TypeLabelName: Type,
	}
}
