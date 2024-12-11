// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"context"
	"strings"

	pkgerrors "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	eslabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/rbac"
)

const (
	// ApmAssociationLabelName marks resources created for an association originating from APM with the APM name.
	ApmAssociationLabelName = "apmassociation.k8s.elastic.co/name"
	// ApmAssociationLabelNamespace marks resources created for an association originating from APM with the APM namespace.
	ApmAssociationLabelNamespace = "apmassociation.k8s.elastic.co/namespace"
	// ApmAssociationLabelType marks resources created for an association originating from APM with the target resource
	// type (e.g. "elasticsearch" or "kibana").
	ApmAssociationLabelType = "apmassociation.k8s.elastic.co/type"
)

func AddApmES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedShortName:       "apm",
		AssociatedObjTemplate:     func() commonv1.Associated { return &apmv1.ApmServer{} },
		ReferencedObjTemplate:     func() client.Object { return &esv1.Elasticsearch{} },
		AssociationType:           commonv1.ElasticsearchAssociationType,
		ReferencedResourceVersion: referencedElasticsearchStatusVersion,
		ExternalServiceURL:        getElasticsearchExternalURL,
		ReferencedResourceNamer:   esv1.ESNamer,
		AssociationName:           "apm-es",
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				ApmAssociationLabelName:      associated.Name,
				ApmAssociationLabelNamespace: associated.Namespace,
				ApmAssociationLabelType:      commonv1.ElasticsearchAssociationType,
			}
		},
		AssociationConfAnnotationNameBase:     commonv1.ElasticsearchConfigAnnotationNameBase,
		AssociationResourceNameLabelName:      eslabel.ClusterNameLabelName,
		AssociationResourceNamespaceLabelName: eslabel.ClusterNamespaceLabelName,

		ElasticsearchUserCreation: &association.ElasticsearchUserCreation{
			ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
				return true, association.AssociationRef(), nil
			},
			UserSecretSuffix: "apm-user",
			ESUserRole:       getAPMElasticsearchRoles,
		},
	})
}

func getElasticsearchExternalURL(c k8s.Client, assoc commonv1.Association) (string, error) {
	esRef := assoc.AssociationRef()
	if !esRef.IsDefined() {
		return "", nil
	}
	es := esv1.Elasticsearch{}
	if err := c.Get(context.Background(), esRef.NamespacedName(), &es); err != nil {
		return "", err
	}
	serviceName := esRef.ServiceName
	if serviceName == "" {
		serviceName = services.ExternalServiceName(es.Name)
	}
	nsn := types.NamespacedName{Name: serviceName, Namespace: es.Namespace}
	return association.ServiceURL(c, nsn, es.Spec.HTTP.Protocol(), "")
}

// getAPMElasticsearchRoles returns for a given version of the APM Server the set of required roles.
func getAPMElasticsearchRoles(associated commonv1.Associated) (string, error) {
	apmServer, ok := associated.(*apmv1.ApmServer)
	if !ok {
		return "", pkgerrors.Errorf(
			"ApmServer expected, got %s/%s",
			associated.GetObjectKind().GroupVersionKind().Group,
			associated.GetObjectKind().GroupVersionKind().Kind,
		)
	}

	v, err := version.Parse(apmServer.Spec.Version)
	if err != nil {
		return "", err
	}

	// 8.7.x and above
	if v.GTE(version.MinFor(8, 7, 0)) {
		return strings.Join([]string{
			user.ApmUserRoleV87, // Retrieve cluster details (e.g. version) and manage apm-* indices
			"apm_system",        // To collect metrics about APM Server
		}, ","), nil
	}

	// 8.0.x and above
	if v.GTE(version.MinFor(8, 0, 0)) {
		return strings.Join([]string{
			user.ApmUserRoleV80, // Retrieve cluster details (e.g. version) and manage apm-* indices
			"apm_system",        // To collect metrics about APM Server
		}, ","), nil
	}

	// 7.5.x and above
	if v.GTE(version.From(7, 5, 0)) {
		return strings.Join([]string{
			user.ApmUserRoleV75, // Retrieve cluster details (e.g. version) and manage apm-* indices
			"ingest_admin",      // Set up index templates
			"apm_system",        // To collect metrics about APM Server
		}, ","), nil
	}

	// 7.1.x to 7.4.x
	if v.GTE(version.From(7, 1, 0)) {
		return strings.Join([]string{
			user.ApmUserRoleV7, // Retrieve cluster details (e.g. version) and manage apm-* indices
			"ingest_admin",     // Set up index templates
			"apm_system",       // To collect metrics about APM Server
		}, ","), nil
	}

	// 6.8
	return strings.Join([]string{
		user.ApmUserRoleV6, // Retrieve cluster details (e.g. version) and manage apm-* indices
		"apm_system",       // To collect metrics about APM Server
	}, ","), nil
}
