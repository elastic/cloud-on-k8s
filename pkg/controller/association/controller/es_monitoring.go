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
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	eslabel "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
)

const (
	// EsAssociationLabelName marks resources created for an association originating from Elasticsearch with the
	// Elasticsearch name.
	EsAssociationLabelName = "esassociation.k8s.elastic.co/name"
	// EsAssociationLabelNamespace marks resources created for an association originating from Elasticsearch with the
	// Elasticsearch namespace.
	EsAssociationLabelNamespace = "esassociation.k8s.elastic.co/namespace"
	// EsAssociationLabelType marks resources created for an association originating from Elasticsearch
	// with the target resource type (e.g. "elasticsearch").
	EsAssociationLabelType = "esassociation.k8s.elastic.co/type"
)

// AddEsMonitoring reconciles an association between two Elasticsearch clusters for Stack Monitoring.
// Beats are configured to collect monitoring metrics and logs data of the associated Elasticsearch and send
// them to the Elasticsearch referenced in the association.
func AddEsMonitoring(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, esMonitoringAssociationInfo())
}

func esMonitoringAssociationInfo() association.AssociationInfo {
	return association.AssociationInfo{
		AssociatedObjTemplate:     func() commonv1.Associated { return &esv1.Elasticsearch{} },
		ReferencedObjTemplate:     func() client.Object { return &esv1.Elasticsearch{} },
		ReferencedResourceVersion: referencedElasticsearchStatusVersion,
		ExternalServiceURL:        getElasticsearchExternalURL,
		AssociationType:           commonv1.EsMonitoringAssociationType,
		ReferencedResourceNamer:   esv1.ESNamer,
		AssociationName:           "es-monitoring",
		AssociatedShortName:       "es-mon",
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				EsAssociationLabelName:      associated.Name,
				EsAssociationLabelNamespace: associated.Namespace,
				EsAssociationLabelType:      commonv1.EsMonitoringAssociationType,
			}
		},
		AssociationConfAnnotationNameBase:     commonv1.ElasticsearchConfigAnnotationNameBase,
		AssociationResourceNameLabelName:      eslabel.ClusterNameLabelName,
		AssociationResourceNamespaceLabelName: eslabel.ClusterNamespaceLabelName,
		ElasticsearchUserCreation: &association.ElasticsearchUserCreation{
			ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
				return true, association.AssociationRef(), nil
			},
			UserSecretSuffix: "beat-es-mon-user",
			ESUserRole: func(associated commonv1.Associated) (string, error) {
				return user.StackMonitoringUserRole, nil
			},
		},
	}
}
