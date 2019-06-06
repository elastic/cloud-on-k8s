// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibanaassociation

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/user"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// AssociationLabelName marks resources created by this controller for easier retrieval.
	AssociationLabelName = "kibanaassociation.k8s.elastic.co/name"
	// AssociationLabelNamespace marks resources created by this controller for easier retrieval.
	AssociationLabelNamespace = "kibanaassociation.k8s.elastic.co/namespace"
)

// NewResourceSelector selects resources labeled as related to the named association.
func NewResourceSelector(name string) labels.Selector {
	return labels.Set(map[string]string{
		AssociationLabelName: name,
	}).AsSelector()
}

func hasBeenCreatedBy(object metav1.Object, kibana v1alpha1.Kibana) bool {
	label := object.GetLabels()
	if name, ok := label[AssociationLabelName]; !ok || name != kibana.Name {
		return false
	}
	if ns, ok := label[AssociationLabelNamespace]; !ok || ns != kibana.Namespace {
		return false
	}
	return true
}

func NewUserLabelSelector(
	namespacedName types.NamespacedName,
) labels.Selector {
	return labels.SelectorFromSet(
		map[string]string{
			AssociationLabelName:      namespacedName.Name,
			AssociationLabelNamespace: namespacedName.Namespace,
			common.TypeLabelName:      user.UserType,
		})
}

// hasExpectedLabels does a left-biased comparison ensuring all key/value pairs in expected exist in actual.
func hasExpectedLabels(expected, actual metav1.Object) bool {
	actualLabels := actual.GetLabels()
	for k, v := range expected.GetLabels() {
		if actualLabels[k] != v {
			return false
		}
	}
	return true
}

// setExpectedLabels set the labels from expected into actual.
func setExpectedLabels(expected, actual metav1.Object) {
	actualLabels := actual.GetLabels()
	if actualLabels == nil {
		actualLabels = make(map[string]string)
	}
	for k, v := range expected.GetLabels() {
		actualLabels[k] = v
	}
	actual.SetLabels(actualLabels)
}
