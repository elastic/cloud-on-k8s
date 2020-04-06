// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
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
		ESUserRole:       esuser.SuperUserBuiltinRole,
	})
}
