// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserverelasticsearchassociation

import (
	"k8s.io/apimachinery/pkg/labels"
)

const (
	// AssociationLabelName marks resources created by this controller for easier retrieval.
	AssociationLabelName = "apmassociation.k8s.elastic.co/name"
)

// NewResourceSelector selects resources labeled as related to the named association.
func NewResourceSelector(name string) labels.Selector {
	return labels.Set(map[string]string{
		AssociationLabelName: name,
	}).AsSelector()
}
