// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	entv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
)

const (
	// EntESAssociationLabelName marks resources created by this controller for easier retrieval.
	EntESAssociationLabelName = "entassociation.k8s.elastic.co/name"
	// EntESAssociationLabelNamespace marks resources created by this controller for easier retrieval.
	EntESAssociationLabelNamespace = "entassociation.k8s.elastic.co/namespace"
)

func AddEntES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate: func() commonv1.Associated { return &entv1beta1.EnterpriseSearch{} },
		AssociationName:       "ent-es",
		AssociatedShortName:   "ent",
		AssociationLabels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				EntESAssociationLabelName:      associated.Name,
				EntESAssociationLabelNamespace: associated.Namespace,
			}
		},
		UserSecretSuffix: "ent-user",
		ESUserRole: func(_ commonv1.Associated) (string, error) {
			return esuser.SuperUserBuiltinRole, nil
		},
	})
}
