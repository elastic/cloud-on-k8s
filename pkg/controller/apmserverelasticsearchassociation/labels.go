// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserverelasticsearchassociation

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/user"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// AssociationLabelName marks resources created by this controller for easier retrieval.
	AssociationLabelName = "apmassociation.k8s.elastic.co/name"
	// AssociationLabelNamespace marks resources created by this controller for easier retrieval.
	AssociationLabelNamespace = "apmassociation.k8s.elastic.co/namespace"
)

// NewResourceSelector selects resources labeled as related to the named association.
// TODO sabo can we remove this?
func NewResourceSelector(name string) labels.Selector {
	return labels.Set(map[string]string{
		AssociationLabelName: name,
	}).AsSelector()
}

// TODO SABO does this exist somewhere else?
func NewResourceLabels(name string) map[string]string {
	return map[string]string{AssociationLabelName: name}
}

func NewUserLabelSelector(
	namespacedName types.NamespacedName,
) client.MatchingLabels {
	return client.MatchingLabels(
		map[string]string{
			AssociationLabelName:      namespacedName.Name,
			AssociationLabelNamespace: namespacedName.Namespace,
			common.TypeLabelName:      user.UserType,
		})
}
