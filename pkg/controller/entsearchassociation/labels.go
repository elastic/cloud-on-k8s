// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package entsearchassociation

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// AssociationLabelName marks resources created by this controller for easier retrieval.
	AssociationLabelName = "entsearchassociation.k8s.elastic.co/name"
	// AssociationLabelNamespace marks resources created by this controller for easier retrieval.
	AssociationLabelNamespace = "entsearchassociation.k8s.elastic.co/namespace"
)

// NewResourceLabels returns the labels to identify an EnterpriseSearch association
func NewResourceLabels(name string) map[string]string {
	return map[string]string{AssociationLabelName: name}
}

func NewUserLabelSelector(namespacedName types.NamespacedName) client.MatchingLabels {
	return map[string]string{
		AssociationLabelName:      namespacedName.Name,
		AssociationLabelNamespace: namespacedName.Namespace,
		common.TypeLabelName:      user.AssociatedUserType,
	}
}
