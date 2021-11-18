// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	entv1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	eslabel "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
)

const (
	// EntESAssociationLabelName marks resources created for an association originating from EnterpriseSearch with the
	// EnterpriseSearch name.
	EntESAssociationLabelName = "entassociation.k8s.elastic.co/name"
	// EntESAssociationLabelNamespace marks resources created for an association originating from EnterpriseSearch with the
	// EnterpriseSearch namespace.
	EntESAssociationLabelNamespace = "entassociation.k8s.elastic.co/namespace"
	// EntESAssociationLabelType marks resources created for an association originating from EnterpriseSearch
	// with the target resource type (e.g. "elasticsearch").
	EntESAssociationLabelType = "entassociation.k8s.elastic.co/type"
)

func AddEntES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate:     func() commonv1.Associated { return &entv1.EnterpriseSearch{} },
		ReferencedObjTemplate:     func() client.Object { return &esv1.Elasticsearch{} },
		ReferencedResourceVersion: referencedElasticsearchStatusVersion,
		AssociationType:           commonv1.ElasticsearchAssociationType,
		ExternalServiceURL:        getElasticsearchExternalURL,
		ReferencedResourceNamer:   esv1.ESNamer,
		AssociationName:           "ent-es",
		AssociatedShortName:       "ent",
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				EntESAssociationLabelName:      associated.Name,
				EntESAssociationLabelNamespace: associated.Namespace,
				EntESAssociationLabelType:      commonv1.ElasticsearchAssociationType,
			}
		},
		AssociationConfAnnotationNameBase:     commonv1.ElasticsearchConfigAnnotationNameBase,
		AssociationResourceNameLabelName:      eslabel.ClusterNameLabelName,
		AssociationResourceNamespaceLabelName: eslabel.ClusterNamespaceLabelName,
		Predicates:                            []predicate.Predicate{common.ManagedNamespacesPredicate(params.ManagedNamespaces)},

		ElasticsearchUserCreation: &association.ElasticsearchUserCreation{
			ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
				return true, association.AssociationRef(), nil
			},
			UserSecretSuffix: "ent-user",
			ESUserRole: func(_ commonv1.Associated) (string, error) {
				return esuser.SuperUserBuiltinRole, nil
			},
		},
	})
}
