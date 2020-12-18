// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	eslabel "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
)

const (
	// KibanaESAssociationLabelName marks resources created by this controller for easier retrieval.
	KibanaESAssociationLabelName = "kibanaassociation.k8s.elastic.co/name"
	// KibanaESAssociationLabelNamespace marks resources created by this controller for easier retrieval.
	KibanaESAssociationLabelNamespace = "kibanaassociation.k8s.elastic.co/namespace"
	// KibanaESAssociationLabelType marks the type of association
	KibanaESAssociationLabelType = "kibanaassociation.k8s.elastic.co/type"

	// KibanaSystemUserBuiltinRole is the name of the built-in role for the Kibana system user.
	KibanaSystemUserBuiltinRole = "kibana_system"
)

func AddKibanaES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate: func() commonv1.Associated { return &kbv1.Kibana{} },
		ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
			return true, association.AssociationRef(), nil
		},
		ReferencedResourceVersion: referencedElasticsearchStatusVersion,
		ExternalServiceURL:        getElasticsearchExternalURL,
		AssociationType:           commonv1.ElasticsearchAssociationType,
		AssociatedNamer:           esv1.ESNamer,
		AssociationName:           "kb-es",
		AssociatedShortName:       "kb",
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				KibanaESAssociationLabelName:      associated.Name,
				KibanaESAssociationLabelNamespace: associated.Namespace,
				KibanaESAssociationLabelType:      commonv1.ElasticsearchAssociationType,
			}
		},
		AssociationConfAnnotationNameBase: commonv1.ElasticsearchConfigAnnotationNameBase,
		UserSecretSuffix:                  "kibana-user",
		ESUserRole: func(associated commonv1.Associated) (string, error) {
			return KibanaSystemUserBuiltinRole, nil
		},
		AssociationResourceNameLabelName:      eslabel.ClusterNameLabelName,
		AssociationResourceNamespaceLabelName: eslabel.ClusterNamespaceLabelName,
	})
}
