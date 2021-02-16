// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
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
	// AgentAssociationLabelName marks resources created by this controller for easier retrieval.
	AgentAssociationLabelName = "agentassociation.k8s.elastic.co/name"
	// AgentAssociationLabelNamespace marks resources created by this controller for easier retrieval.
	AgentAssociationLabelNamespace = "agentassociation.k8s.elastic.co/namespace"
	// AgentAssociationLabelType marks the type of association
	AgentAssociationLabelType = "agentassociation.k8s.elastic.co/type"
)

func AddAgentES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociationType:       commonv1.ElasticsearchAssociationType,
		AssociatedObjTemplate: func() commonv1.Associated { return &agentv1alpha1.Agent{} },
		ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
			return true, association.AssociationRef(), nil
		},
		ReferencedResourceVersion: referencedElasticsearchStatusVersion,
		ExternalServiceURL:        getElasticsearchExternalURL,
		AssociatedNamer:           esv1.ESNamer,
		AssociationName:           "agent-es",
		AssociatedShortName:       "agent",
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				AgentAssociationLabelName:      associated.Name,
				AgentAssociationLabelNamespace: associated.Namespace,
				AgentAssociationLabelType:      commonv1.ElasticsearchAssociationType,
			}
		},
		AssociationConfAnnotationNameBase: commonv1.ElasticsearchConfigAnnotationNameBase,
		UserSecretSuffix:                  "agent-user",
		ESUserRole: func(associated commonv1.Associated) (string, error) {
			return "superuser", nil
		},
		AssociationResourceNameLabelName:      eslabel.ClusterNameLabelName,
		AssociationResourceNamespaceLabelName: eslabel.ClusterNamespaceLabelName,
	})
}
