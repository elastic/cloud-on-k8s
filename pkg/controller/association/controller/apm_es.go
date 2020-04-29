// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	"strings"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	pkgerrors "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// ApmESAssociationLabelName marks resources created by this controller for easier retrieval.
	ApmESAssociationLabelName = "apmassociation.k8s.elastic.co/name"
	// ApmESAssociationLabelNamespace marks resources created by this controller for easier retrieval.
	ApmESAssociationLabelNamespace = "apmassociation.k8s.elastic.co/namespace"
)

func AddApmES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedShortName:   "apm",
		AssociatedObjTemplate: func() commonv1.Associated { return &apmv1.ApmServer{} },
		AssociationName:       "apm-es",
		AssociationLabels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				ApmESAssociationLabelName:      associated.Name,
				ApmESAssociationLabelNamespace: associated.Namespace,
			}
		},
		UserSecretSuffix: "apm-user",
		ESUserRole:       getRoles,
	})
}

// getRoles returns for a given version of the APM Server the set of required roles.
func getRoles(associated commonv1.Associated) (string, error) {
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
