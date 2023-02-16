// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"strings"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	eslabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/rbac"
)

const (
	// LogstashAssociationLabelName marks resources created for an association originating from Logstash with the
	// Logstash name.
	LogstashAssociationLabelName = "logstashassociation.k8s.elastic.co/name"
	// LogstashAssociationLabelNamespace marks resources created for an association originating from Logstash with the
	// Logstash namespace.
	LogstashAssociationLabelNamespace = "logstashassociation.k8s.elastic.co/namespace"
	// LogstashAssociationLabelType marks resources created for an association originating from Logstash
	// with the target resource type (e.g. "elasticsearch").
	LogstashAssociationLabelType = "logstashassociation.k8s.elastic.co/type"
	// LogstashSystemUserBuiltinRole is the name of the built-in role for the Logstash system user.
	LogstashSystemUserBuiltinRole = "logstash_system"
	// LogstashAdminUserBuiltinRole is the name of the built-in role for the Logstash admin user.
	LogstashAdminUserBuiltinRole = "logstash_admin"
)

func AddLogstashES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociationType:           commonv1.ElasticsearchAssociationType,
		AssociatedObjTemplate:     func() commonv1.Associated { return &logstashv1alpha1.Logstash{} },
		ReferencedObjTemplate:     func() client.Object { return &esv1.Elasticsearch{} },
		ReferencedResourceVersion: referencedElasticsearchStatusVersion,
		ExternalServiceURL:        getElasticsearchExternalURL,
		ReferencedResourceNamer:   esv1.ESNamer,
		AssociationName:           "logstash-es",
		AssociatedShortName:       "logstash",
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				LogstashAssociationLabelName:      associated.Name,
				LogstashAssociationLabelNamespace: associated.Namespace,
				LogstashAssociationLabelType:      commonv1.ElasticsearchAssociationType,
			}
		},
		AssociationConfAnnotationNameBase:     commonv1.ElasticsearchConfigAnnotationNameBase,
		AssociationResourceNameLabelName:      eslabel.ClusterNameLabelName,
		AssociationResourceNamespaceLabelName: eslabel.ClusterNamespaceLabelName,

		ElasticsearchUserCreation: &association.ElasticsearchUserCreation{
			ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
				return true, association.AssociationRef(), nil
			},
			UserSecretSuffix: "logstash-user",
			ESUserRole: func(associated commonv1.Associated) (string, error) {
				return strings.Join([]string{LogstashAdminUserBuiltinRole, LogstashSystemUserBuiltinRole}, ","), nil
			},
		},
	})
}
