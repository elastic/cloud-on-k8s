// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	eslabel "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
)

const (
	// BeatESAssociationLabelName marks resources created by this controller for easier retrieval.
	BeatESAssociationLabelName = "beatassociation.k8s.elastic.co/name"

	// BeatESAssociationLabelNamespace marks resources created by this controller for easier retrieval.
	BeatESAssociationLabelNamespace = "beatassociation.k8s.elastic.co/namespace"
)

func AddBeatES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociationObjTemplate: func() commonv1.Association { return &beatv1beta1.Beat{} },
		ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
			return true, association.AssociationRef(), nil
		},
		ExternalServiceURL:  getElasticsearchExternalURL,
		AssociatedNamer:     esv1.ESNamer,
		AssociationName:     "beat-es",
		AssociatedShortName: "beat",
		AssociationLabels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				BeatESAssociationLabelName:      associated.Name,
				BeatESAssociationLabelNamespace: associated.Namespace,
			}
		},
		UserSecretSuffix:  "beat-user",
		CASecretLabelName: eslabel.ClusterNameLabelName,
		ESUserRole: func(commonv1.Associated) (string, error) {
			return esuser.SuperUserBuiltinRole, nil
		},
	})
}
