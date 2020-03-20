// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserverelasticsearchassociation

import (
	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// AssociationLabelName marks resources created by this controller for easier retrieval.
	AssociationLabelName = "apmassociation.k8s.elastic.co/name"
	// AssociationLabelNamespace marks resources created by this controller for easier retrieval.
	AssociationLabelNamespace = "apmassociation.k8s.elastic.co/namespace"
)

func associationLabels(apmServer *apmv1.ApmServer) map[string]string {
	return map[string]string{
		AssociationLabelName:      apmServer.Name,
		AssociationLabelNamespace: apmServer.Namespace,
	}
}

func newUserLabelSelector(
	namespacedName types.NamespacedName,
) client.MatchingLabels {
	return client.MatchingLabels(
		map[string]string{
			AssociationLabelName:      namespacedName.Name,
			AssociationLabelNamespace: namespacedName.Namespace,
			common.TypeLabelName:      esuser.AssociatedUserType,
		})
}
