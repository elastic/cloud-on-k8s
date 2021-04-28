// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	eslabel "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// MapsESAssociationLabelName marks resources created by this controller for easier retrieval.
	MapsESAssociationLabelName = "mapsassociation.k8s.elastic.co/name"
	// MapsESAssociationLabelNamespace marks resources created by this controller for easier retrieval.
	MapsESAssociationLabelNamespace = "mapsassociation.k8s.elastic.co/namespace"
	// MapsESAssociationLabelType marks the type of association
	MapsESAssociationLabelType = "mapsassociation.k8s.elastic.co/type"

	// MapsSystemUserBuiltinRole is the name of the built-in role for the Maps system user.
	MapsSystemUserBuiltinRole = user.ProbeUserRole
)

func AddMapsES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate: func() commonv1.Associated { return &emsv1alpha1.ElasticMapsServer{} },
		ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
			return true, association.AssociationRef(), nil
		},
		ReferencedResourceVersion: referencedElasticsearchStatusVersion,
		ExternalServiceURL:        getElasticsearchExternalURL,
		AssociationType:           commonv1.ElasticsearchAssociationType,
		AssociatedNamer:           esv1.ESNamer,
		AssociationName:           "ems-es",
		AssociatedShortName:       "ems",
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				MapsESAssociationLabelName:      associated.Name,
				MapsESAssociationLabelNamespace: associated.Namespace,
				MapsESAssociationLabelType:      commonv1.ElasticsearchAssociationType,
			}
		},
		AssociationConfAnnotationNameBase: commonv1.ElasticsearchConfigAnnotationNameBase,
		UserSecretSuffix:                  "maps-user",
		ESUserRole: func(associated commonv1.Associated) (string, error) {
			return MapsSystemUserBuiltinRole, nil
		},
		AssociationResourceNameLabelName:      eslabel.ClusterNameLabelName,
		AssociationResourceNamespaceLabelName: eslabel.ClusterNamespaceLabelName,
	})
}
