// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

const (
	// LogstashAssociationLabelName marks resources created for an association originating from Logstash with the
	// Logstash name.
	LogstashAssociationLabelName = "logstashassociation.k8s.elastic.co/name"
	// LogstashAssociationLabelNamespace marks resources created for an association originating from Logstash with the
	// Logstash namespace.
	LogstashAssociationLabelNamespace = "logstashassociation.k8s.elastic.co/namespace"
	// LogstashAssociationLabelType marks resources created for an association originating from Logstash
	// with the target resource type (e.g. "elasticsearch").
	LogstashAssociationLabelType = "logstashassociation.k8s.elastic.co/type"
)
