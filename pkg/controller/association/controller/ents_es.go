// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
)

const (
	// EntSearchESAssociationLabelName marks resources created by this controller for easier retrieval.
	EntSearchESAssociationLabelName = "entsearchassociation.k8s.elastic.co/name"
	// EntSearchESLabelNamespace marks resources created by this controller for easier retrieval.
	EntSearchESAssociationLabelNamespace = "entsearchassociation.k8s.elastic.co/namespace"
)

func AddEntSearchES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate: func() commonv1.Associated { return &entsv1beta1.EnterpriseSearch{} },
		AssociationName:       "ents-es",
		AssociatedShortName:   "ents",
		AssociationLabels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				EntSearchESAssociationLabelName:      associated.Name,
				EntSearchESAssociationLabelNamespace: associated.Namespace,
			}
		},
		UserSecretSuffix: "ents-user",
		ESUserRole:       esuser.SuperUserBuiltinRole,
	})
}
