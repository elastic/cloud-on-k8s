// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

const (
	// KibanaNameLabelName used to represent a Kibana in k8s resources
	KibanaNameLabelName = "kibana.k8s.elastic.co/name"

	// KibanaNamespaceLabelName used to represent a Kibana in k8s resources
	KibanaNamespaceLabelName = "kibana.k8s.elastic.co/namespace"

	// KibanaVersionLabelName used to propagate Kibana version from the spec to the pods
	KibanaVersionLabelName = "kibana.k8s.elastic.co/version"

	// Type represents the Kibana type
	Type = "kibana"
)
