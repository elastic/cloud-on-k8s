// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	beatv1b1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	eslabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/rbac"
)

// AddBeatMonitoring reconciles an association between Beat and Elasticsearch clusters for Stack Monitoring.
// Beat is configured with sidecars to send its monitoring data to the Elasticsearch referenced in the association.
func AddBeatMonitoring(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate:     func() commonv1.Associated { return &beatv1b1.Beat{} },
		ReferencedObjTemplate:     func() client.Object { return &esv1.Elasticsearch{} },
		ReferencedResourceVersion: referencedElasticsearchStatusVersion,
		ExternalServiceURL:        getElasticsearchExternalURL,
		AssociationType:           commonv1.BeatMonitoringAssociationType,
		ReferencedResourceNamer:   esv1.ESNamer,
		AssociationName:           "beat-monitoring",
		AssociatedShortName:       "beat-mon",
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				BeatAssociationLabelName:      associated.Name,
				BeatAssociationLabelNamespace: associated.Namespace,
				BeatAssociationLabelType:      commonv1.BeatMonitoringAssociationType,
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
	})
}
