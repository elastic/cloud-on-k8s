// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

const (
	// TypeLabelValue represents the Logstash type.
	TypeLabelValue = "logstash"

	// NameLabelName used to represent an Logstash in k8s resources
	NameLabelName = "logstash.k8s.elastic.co/name"

	// NamespaceLabelName used to represent an Logstash in k8s resources
	NamespaceLabelName = "logstash.k8s.elastic.co/namespace"
)
