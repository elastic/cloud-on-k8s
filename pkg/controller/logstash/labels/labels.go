// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package labels

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
)

const (
	// TypeLabelValue represents the Logstash type.
	TypeLabelValue = "logstash"

	// NameLabelName used to represent a Logstash in k8s resources
	NameLabelName = "logstash.k8s.elastic.co/name"

	// NamespaceLabelName used to represent a Logstash in k8s resources
	NamespaceLabelName = "logstash.k8s.elastic.co/namespace"

	// StatefulSetNameLabelName used to store the name of the statefulset.
	StatefulSetNameLabelName = "logstash.k8s.elastic.co/statefulset-name"
)

// NewLabels returns the set of common labels for an Elastic Logstash.
func NewLabels(logstash logstashv1alpha1.Logstash) map[string]string {
	return map[string]string{
		commonv1.TypeLabelName: TypeLabelValue,
		NameLabelName:          logstash.Name,
	}
}

// NewLabelSelectorForLogstash returns a labels.Selector that matches the labels as constructed by NewLabels
func NewLabelSelectorForLogstash(ls logstashv1alpha1.Logstash) client.MatchingLabels {
	return client.MatchingLabels(map[string]string{commonv1.TypeLabelName: TypeLabelValue, NameLabelName: ls.Name})
}
