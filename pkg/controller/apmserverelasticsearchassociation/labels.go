// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserverelasticsearchassociation

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/user"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// AssociationLabelName marks resources created by this controller for easier retrieval.
	AssociationLabelName = "apmassociation.k8s.elastic.co/name"
	// AssociationLabelNamespace marks resources created by this controller for easier retrieval.
	AssociationLabelNamespace = "apmassociation.k8s.elastic.co/namespace"
)

var createdByApmServer association.CreatedBy = func(created, creator metav1.Object) bool {
	labels := created.GetLabels()
	if name, ok := labels[AssociationLabelName]; !ok || name != creator.GetName() {
		return false
	}
	if ns, ok := labels[AssociationLabelNamespace]; !ok || ns != creator.GetNamespace() {
		return false
	}
	return true
}

// NewResourceLabels returns the labels to identify an APM association
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
