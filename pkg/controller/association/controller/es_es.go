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
	// EsEsAssociationLabelName marks resources created by this controller for easier retrieval.
	EsEsAssociationLabelName = "esesassociation.k8s.elastic.co/name"
	// EsEsAssociationLabelNamespace marks resources created by this controller for easier retrieval.
	EsEsAssociationLabelNamespace = "esesassociation.k8s.elastic.co/namespace"
	// EsEsAssociationLabelType marks the type of association.
	EsEsAssociationLabelType = "esesassociation.k8s.elastic.co/type"

	// BeatBuiltinRole is the name of the built-in role for the Metricbeat/Filebeat system user.
	BeatBuiltinRole = "superuser" // FIXME: create a dedicated role?
)

func AddEsEs(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate: func() commonv1.Associated { return &esv1.Elasticsearch{} },
		ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
			return true, association.AssociationRef(), nil
		},
		ReferencedResourceVersion: referencedElasticsearchStatusVersion,
		ExternalServiceURL:        getElasticsearchExternalURL,
		AssociationType:           commonv1.ElasticsearchAssociationType,
		AssociatedNamer:           esv1.ESNamer,
		AssociationName:           "es-es",
		AssociatedShortName:       "es",
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				EsEsAssociationLabelName:      associated.Name,
				EsEsAssociationLabelNamespace: associated.Namespace,
				EsEsAssociationLabelType:      commonv1.ElasticsearchAssociationType,
			}
		},
		AssociationConfAnnotationNameBase: commonv1.ElasticsearchConfigAnnotationNameBase,
		UserSecretSuffix:                  "beat-user",
		ESUserRole: func(associated commonv1.Associated) (string, error) {
			return BeatBuiltinRole, nil
		},
		AssociationResourceNameLabelName:      eslabel.ClusterNameLabelName,
		AssociationResourceNamespaceLabelName: eslabel.ClusterNamespaceLabelName,
	})
}
