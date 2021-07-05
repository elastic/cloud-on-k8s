// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	// AgentAssociationLabelName marks resources created for an association originating from Agent with the
	// Agent name.
	AgentAssociationLabelName = "agentassociation.k8s.elastic.co/name"
	// AgentAssociationLabelNamespace marks resources created for an association originating from Agent with the
	// Agent namespace.
	AgentAssociationLabelNamespace = "agentassociation.k8s.elastic.co/namespace"
	// AgentAssociationLabelType marks resources created for an association originating from Agent
	// with the target resource type (e.g. "elasticsearch").
	AgentAssociationLabelType = "agentassociation.k8s.elastic.co/type"
)

func AddAgentES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociationType:           commonv1.ElasticsearchAssociationType,
		AssociatedObjTemplate:     func() commonv1.Associated { return &agentv1alpha1.Agent{} },
		ReferencedObjTemplate:     func() client.Object { return &esv1.Elasticsearch{} },
		ReferencedResourceVersion: referencedElasticsearchStatusVersion,
		ExternalServiceURL:        getElasticsearchExternalURL,
		ReferencedResourceNamer:   esv1.ESNamer,
		AssociationName:           "agent-es",
		AssociatedShortName:       "agent",
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				AgentAssociationLabelName:      associated.Name,
				AgentAssociationLabelNamespace: associated.Namespace,
				AgentAssociationLabelType:      commonv1.ElasticsearchAssociationType,
			}
		},
		AssociationConfAnnotationNameBase:     commonv1.ElasticsearchConfigAnnotationNameBase,
		AssociationResourceNameLabelName:      eslabel.ClusterNameLabelName,
		AssociationResourceNamespaceLabelName: eslabel.ClusterNamespaceLabelName,

		ElasticsearchUserCreation: &association.ElasticsearchUserCreation{
			ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
				return true, association.AssociationRef(), nil
			},
			UserSecretSuffix: "agent-user",
			ESUserRole: func(associated commonv1.Associated) (string, error) {
				return "superuser", nil
			},
		},
	})
}
