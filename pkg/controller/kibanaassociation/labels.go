// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibanaassociation

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/user"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	// "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// AssociationLabelName marks resources created by this controller for easier retrieval.
	AssociationLabelName = "kibanaassociation.k8s.elastic.co/name"
	// AssociationLabelNamespace marks resources created by this controller for easier retrieval.
	AssociationLabelNamespace = "kibanaassociation.k8s.elastic.co/namespace"
)

// NewResourceSelector selects resources labeled as related to the named association.
func NewResourceSelector(name string) client.MatchingLabels {
	return client.MatchingLabels(map[string]string{
		AssociationLabelName: name,
	})
}

func hasBeenCreatedBy(object metav1.Object, kibana v1alpha1.Kibana) bool {
	labels := object.GetLabels()
	if name, ok := labels[AssociationLabelName]; !ok || name != kibana.Name {
		return false
	}
	if ns, ok := labels[AssociationLabelNamespace]; !ok || ns != kibana.Namespace {
		return false
	}
	return true
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
