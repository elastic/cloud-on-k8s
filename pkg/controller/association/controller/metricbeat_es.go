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
	// MetricbeatESAssociationLabelName marks resources created by this controller for easier retrieval.
	MetricbeatESAssociationLabelName = "metricbeatassociation.k8s.elastic.co/name"
	// MetricbeatESAssociationLabelNamespace marks resources created by this controller for easier retrieval.
	MetricbeatESAssociationLabelNamespace = "metricbeatassociation.k8s.elastic.co/namespace"
	// MetricbeatESAssociationLabelType marks the type of association.
	MetricbeatESAssociationLabelType = "metricbeatassociation.k8s.elastic.co/type"

	// MetricbeatBuiltinRole is the name of the built-in role for the Metricbeat system user.
	MetricbeatBuiltinRole = "superuser" // FIXME: create a dedicated role?
)

func AddMetricbeatES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate: func() commonv1.Associated { return &esv1.Elasticsearch{} },
		ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
			return true, association.AssociationRef(), nil
		},
		ReferencedResourceVersion: referencedElasticsearchStatusVersion,
		ExternalServiceURL:        getElasticsearchExternalURL,
		AssociationType:           commonv1.MetricbeatAssociationType,
		AssociatedNamer:           esv1.ESNamer,
		AssociationName:           "metricbeat-es",
		AssociatedShortName:       "metricbeat",
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				MetricbeatESAssociationLabelName:      associated.Name,
				MetricbeatESAssociationLabelNamespace: associated.Namespace,
				MetricbeatESAssociationLabelType:      commonv1.MetricbeatAssociationType,
			}
		},
		AssociationConfAnnotationNameBase: commonv1.MetricbeatConfigAnnotationNameBase,
		UserSecretSuffix:                  "metricbeat-user",
		ESUserRole: func(associated commonv1.Associated) (string, error) {
			return MetricbeatBuiltinRole, nil
		},
		AssociationResourceNameLabelName:      eslabel.ClusterNameLabelName,
		AssociationResourceNamespaceLabelName: eslabel.ClusterNamespaceLabelName,
	})
}
