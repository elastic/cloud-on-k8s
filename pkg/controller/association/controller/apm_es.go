// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	"context"
	"strings"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	eslabel "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	pkgerrors "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// ApmAssociationLabelName marks resources created for an association originating from APM.
	ApmAssociationLabelName = "apmassociation.k8s.elastic.co/name"
	// ApmAssociationLabelNamespace marks resources created for an association originating from APM.
	ApmAssociationLabelNamespace = "apmassociation.k8s.elastic.co/namespace"
	// ApmAssociationLabelType marks resources created for an association originating from APM.
	ApmAssociationLabelType = "apmassociation.k8s.elastic.co/type"
)

func AddApmES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedShortName:   "apm",
		AssociatedObjTemplate: func() commonv1.Associated { return &apmv1.ApmServer{} },
		AssociationType:       commonv1.ElasticsearchAssociationType,
		ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
			return true, association.AssociationRef(), nil
		},
		ReferencedResourceVersion: referencedElasticsearchStatusVersion,
		ExternalServiceURL:        getElasticsearchExternalURL,
		AssociatedNamer:           esv1.ESNamer,
		AssociationName:           "apm-es",
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				ApmAssociationLabelName:      associated.Name,
				ApmAssociationLabelNamespace: associated.Namespace,
				ApmAssociationLabelType:      commonv1.ElasticsearchAssociationType,
			}
		},
		AssociationConfAnnotationNameBase:     commonv1.ElasticsearchConfigAnnotationNameBase,
		UserSecretSuffix:                      "apm-user",
		ESUserRole:                            getAPMElasticsearchRoles,
		AssociationResourceNameLabelName:      eslabel.ClusterNameLabelName,
		AssociationResourceNamespaceLabelName: eslabel.ClusterNamespaceLabelName,
	})
}

func getElasticsearchExternalURL(c k8s.Client, association commonv1.Association) (string, error) {
	esRef := association.AssociationRef()
	if !esRef.IsDefined() {
		return "", nil
	}
	es := esv1.Elasticsearch{}
	if err := c.Get(context.Background(), esRef.NamespacedName(), &es); err != nil {
		return "", err
	}
	return services.ExternalServiceURL(es), nil
}

// referencedElasticsearchStatusVersion returns the currently running version of Elasticsearch
// reported in its status.
func referencedElasticsearchStatusVersion(c k8s.Client, esRef types.NamespacedName) (string, error) {
	var es esv1.Elasticsearch
	if err := c.Get(context.Background(), esRef, &es); err != nil {
		return "", err
	}
	return es.Status.Version, nil
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

	// 7.5.x and above
	if v.IsSameOrAfter(version.From(7, 5, 0)) {
		return strings.Join([]string{
			user.ApmUserRoleV75, // Retrieve cluster details (e.g. version) and manage apm-* indices
			"ingest_admin",      // Set up index templates
			"apm_system",        // To collect metrics about APM Server
		}, ","), nil
	}

	// 7.1.x to 7.4.x
	if v.IsSameOrAfter(version.From(7, 1, 0)) {
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
