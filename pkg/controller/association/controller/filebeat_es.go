// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	eslabel "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
)

const (
	// FilebeatESAssociationLabelName marks resources created by this controller for easier retrieval.
	FilebeatESAssociationLabelName = "filebeatassociation.k8s.elastic.co/name"
	// FilebeatESAssociationLabelNamespace marks resources created by this controller for easier retrieval.
	FilebeatESAssociationLabelNamespace = "filebeatassociation.k8s.elastic.co/namespace"
	// FilebeatESAssociationLabelType marks the type of association.
	FilebeatESAssociationLabelType = "filebeatassociation.k8s.elastic.co/type"

	// FilebeatBuiltinRole is the name of the built-in role for the Filebeat system user.
	FilebeatBuiltinRole = "superuser" // FIXME: create a dedicated role?
)

func AddFilebeatES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate: func() commonv1.Associated { return &esv1.Elasticsearch{} },
		ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
			return true, association.AssociationRef(), nil
		},
		ReferencedResourceVersion: referencedElasticsearchStatusVersion,
		ExternalServiceURL:        getElasticsearchExternalURL,
		AssociationType:           commonv1.FilebeatAssociationType,
		AssociatedNamer:           esv1.ESNamer,
		AssociationName:           "filebeat-es",
		AssociatedShortName:       "filebeat",
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				FilebeatESAssociationLabelName:      associated.Name,
				FilebeatESAssociationLabelNamespace: associated.Namespace,
				FilebeatESAssociationLabelType:      commonv1.FilebeatAssociationType,
			}
		},
		AssociationConfAnnotationNameBase: commonv1.FilebeatConfigAnnotationNameBase,
		UserSecretSuffix:                  "filebeat-user",
		ESUserRole: func(associated commonv1.Associated) (string, error) {
			return FilebeatBuiltinRole, nil
		},
		AssociationResourceNameLabelName:      eslabel.ClusterNameLabelName,
		AssociationResourceNamespaceLabelName: eslabel.ClusterNamespaceLabelName,
	})
}
