// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	eslabel "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
)

const (
	// MapsESAssociationLabelName marks resources created for an association originating from Maps with the
	// Maps name.
	MapsESAssociationLabelName = "mapsassociation.k8s.elastic.co/name"
	// MapsESAssociationLabelNamespace marks resources created for an association originating from Maps with the
	// Maps namespace.
	MapsESAssociationLabelNamespace = "mapsassociation.k8s.elastic.co/namespace"
	// MapsESAssociationLabelType marks resources created for an association originating from Maps
	// with the target resource type (e.g. "elasticsearch").
	MapsESAssociationLabelType = "mapsassociation.k8s.elastic.co/type"

	// MapsSystemUserBuiltinRole is the name of the built-in role for the Maps system user.
	MapsSystemUserBuiltinRole = user.ProbeUserRole
)

func AddMapsES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate:     func() commonv1.Associated { return &emsv1alpha1.ElasticMapsServer{} },
		ReferencedObjTemplate:     func() client.Object { return &esv1.Elasticsearch{} },
		ReferencedResourceVersion: referencedElasticsearchStatusVersion,
		ExternalServiceURL:        getElasticsearchExternalURL,
		AssociationType:           commonv1.ElasticsearchAssociationType,
		ReferencedResourceNamer:   esv1.ESNamer,
		AssociationName:           "ems-es",
		AssociatedShortName:       "ems",
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				MapsESAssociationLabelName:      associated.Name,
				MapsESAssociationLabelNamespace: associated.Namespace,
				MapsESAssociationLabelType:      commonv1.ElasticsearchAssociationType,
			}
		},
		AssociationConfAnnotationNameBase:     commonv1.ElasticsearchConfigAnnotationNameBase,
		AssociationResourceNameLabelName:      eslabel.ClusterNameLabelName,
		AssociationResourceNamespaceLabelName: eslabel.ClusterNamespaceLabelName,

		ElasticsearchUserCreation: &association.ElasticsearchUserCreation{
			ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
				return true, association.AssociationRef(), nil
			},
			UserSecretSuffix: "maps-user",
			ESUserRole: func(associated commonv1.Associated) (string, error) {
				return MapsSystemUserBuiltinRole, nil
			},
		},
	})
}
