// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	eslabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/rbac"
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
func referencedElasticsearchStatusVersion(c k8s.Client, esRef commonv1.ObjectSelector) (string, error) {
	if esRef.IsExternal() {
		info, err := association.GetUnmanagedAssociationConnectionInfoFromSecret(c, esRef)
		if err != nil {
			return "", err
		}
		ver, err := info.Version("/", "{ .version.number }")
		if err != nil {
			return "", err
		}
		return ver, nil
	}

	var es esv1.Elasticsearch
	err := c.Get(context.Background(), esRef.NamespacedName(), &es)
	if err != nil {
		return "", err
	}
	return es.Status.Version, nil
}
