// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	pkgerrors "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user"
	kblabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/rbac"
)

func AddAgentKibana(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate:     func() commonv1.Associated { return &agentv1alpha1.Agent{} },
		ReferencedObjTemplate:     func() client.Object { return &kbv1.Kibana{} },
		ExternalServiceURL:        getKibanaExternalURL,
		ReferencedResourceVersion: referencedKibanaStatusVersion,
		ReferencedResourceNamer:   kbv1.KBNamer,
		AssociationName:           "agent-kibana",
		AssociatedShortName:       "agent",
		AssociationType:           commonv1.KibanaAssociationType,
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				AgentAssociationLabelName:      associated.Name,
				AgentAssociationLabelNamespace: associated.Namespace,
				AgentAssociationLabelType:      commonv1.KibanaAssociationType,
			}
		},
		AssociationConfAnnotationNameBase:     commonv1.KibanaConfigAnnotationNameBase,
		AssociationResourceNameLabelName:      kblabel.KibanaNameLabelName,
		AssociationResourceNamespaceLabelName: kblabel.KibanaNamespaceLabelName,

		ElasticsearchUserCreation: &association.ElasticsearchUserCreation{
			ElasticsearchRef: getElasticsearchFromKibana,
			UserSecretSuffix: "agent-kb-user",
			ESUserRole: func(associated commonv1.Associated) (string, error) {
				agent, ok := associated.(*agentv1alpha1.Agent)
				if !ok {
					return "", pkgerrors.Errorf(
						"Agent expected, got %s/%s",
						associated.GetObjectKind().GroupVersionKind().Group,
						associated.GetObjectKind().GroupVersionKind().Kind,
					)
				}
				v, err := version.Parse(agent.Spec.Version)
				if err != nil {
					return "", err
				}
				// Fleet API can only be used as a non-superuser as of 8.1.0 https://github.com/elastic/kibana/issues/108252
				if v.LT(version.MinFor(8, 1, 0)) {
					return "superuser", nil
				}
				return user.FleetAdminUserRole, nil
			},
		},
	})
}
