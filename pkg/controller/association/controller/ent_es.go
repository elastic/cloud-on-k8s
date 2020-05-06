// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	entv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// EntESAssociationLabelName marks resources created by this controller for easier retrieval.
	EntESAssociationLabelName = "entassociation.k8s.elastic.co/name"
	// EntESAssociationLabelNamespace marks resources created by this controller for easier retrieval.
	EntESAssociationLabelNamespace = "entassociation.k8s.elastic.co/namespace"
)

func AddEntES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociationObjTemplate: func() commonv1.Association { return &entv1beta1.EnterpriseSearch{} },
		ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
			return true, association.AssociationRef().WithDefaultNamespace(association.GetNamespace()), nil
		},
		ExternalServiceURL:  getElasticsearchExternalURL,
		AssociatedNamer:     esv1.ESNamer,
		AssociationName:     "ent-es",
		AssociatedShortName: "ent",
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
