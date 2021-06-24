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
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
)

const (
	// EsAssociationLabelName marks resources created by this controller for easier retrieval.
	EsAssociationLabelName = "esassociation.k8s.elastic.co/name"
	// EsAssociationLabelNamespace marks resources created by this controller for easier retrieval.
	EsAssociationLabelNamespace = "esassociation.k8s.elastic.co/namespace"
	// EsAssociationLabelType marks the type of association.
	EsAssociationLabelType = "esassociation.k8s.elastic.co/type"

	EsMonitoringAssociationType = "es-monitoring"
)

// AddEsMonitoring reconciles an association between two Elasticsearch clusters for Stack Monitoring.
// Beats are configured to collect monitoring metrics and logs data of the associated Elasticsearch and send
// them to the Elasticsearch referenced in the association.
func AddEsMonitoring(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate: func() commonv1.Associated { return &esv1.Elasticsearch{} },
		ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
			return true, association.AssociationRef(), nil
		},
		ReferencedResourceVersion: referencedElasticsearchStatusVersion,
		ExternalServiceURL:        getElasticsearchExternalURL,
		AssociationType:           commonv1.EsMonitoringAssociationType,
		AssociatedNamer:           esv1.ESNamer,
		AssociationName:           "es-monitoring",
		AssociatedShortName:       "es-mon",
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				EsAssociationLabelName:      associated.Name,
				EsAssociationLabelNamespace: associated.Namespace,
				EsAssociationLabelType:      EsMonitoringAssociationType,
			}
		},
		AssociationConfAnnotationNameBase: commonv1.ElasticsearchConfigAnnotationNameBase,
		UserSecretSuffix:                  "beat-es-mon-user",
		ESUserRole: func(associated commonv1.Associated) (string, error) {
			return user.StackMonitoringUserRole, nil
		},
		AssociationResourceNameLabelName:      eslabel.ClusterNameLabelName,
		AssociationResourceNamespaceLabelName: eslabel.ClusterNamespaceLabelName,
	})
}
