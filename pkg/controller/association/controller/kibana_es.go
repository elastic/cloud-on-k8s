// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	"context"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	eslabel "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// KibanaAssociationLabelName marks resources created for an association originating from Kibana with the
	// Kibana name.
	KibanaAssociationLabelName = "kibanaassociation.k8s.elastic.co/name"
	// KibanaAssociationLabelNamespace marks resources created for an association originating from Kibana with the
	// Kibana namespace.
	KibanaAssociationLabelNamespace = "kibanaassociation.k8s.elastic.co/namespace"
	// KibanaAssociationLabelType marks resources created for an association originating from Kibana
	// with the target resource type (e.g. "elasticsearch" or "ent).
	KibanaAssociationLabelType = "kibanaassociation.k8s.elastic.co/type"

	// KibanaSystemUserBuiltinRole is the name of the built-in role for the Kibana system user.
	KibanaSystemUserBuiltinRole = "kibana_system"
)

func AddKibanaES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate:     func() commonv1.Associated { return &kbv1.Kibana{} },
		ReferencedObjTemplate:     func() client.Object { return &esv1.Elasticsearch{} },
		ReferencedResourceVersion: referencedElasticsearchStatusVersion,
		ExternalServiceURL:        getElasticsearchExternalURL,
		AssociationType:           commonv1.ElasticsearchAssociationType,
		ReferencedResourceNamer:   esv1.ESNamer,
		AssociationName:           "kb-es",
		AssociatedShortName:       "kb",
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				KibanaAssociationLabelName:      associated.Name,
				KibanaAssociationLabelNamespace: associated.Namespace,
				KibanaAssociationLabelType:      commonv1.ElasticsearchAssociationType,
			}
		},
		AssociationConfAnnotationNameBase:     commonv1.ElasticsearchConfigAnnotationNameBase,
		AssociationResourceNameLabelName:      eslabel.ClusterNameLabelName,
		AssociationResourceNamespaceLabelName: eslabel.ClusterNamespaceLabelName,

		ElasticsearchUserCreation: &association.ElasticsearchUserCreation{
			ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
				return true, association.AssociationRef(), nil
			},
			UserSecretSuffix: "kibana-user",
			ESUserRole: func(associated commonv1.Associated) (string, error) {
				return KibanaSystemUserBuiltinRole, nil
			},
		},
	})
}

// referencedElasticsearchStatusVersion returns the currently running version of Elasticsearch
// reported in its status.
func referencedElasticsearchStatusVersion(c k8s.Client, esRef types.NamespacedName) (string, error) {
	var es esv1.Elasticsearch
	err := c.Get(context.Background(), esRef, &es)
	if err != nil {
		return "", err
	}
	return es.Status.Version, nil
}
