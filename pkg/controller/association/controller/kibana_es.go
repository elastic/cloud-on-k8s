// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	eslabel "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// KibanaESAssociationLabelName marks resources created by this controller for easier retrieval.
	KibanaESAssociationLabelName = "kibanaassociation.k8s.elastic.co/name"
	// KibanaESAssociationLabelNamespace marks resources created by this controller for easier retrieval.
	KibanaESAssociationLabelNamespace = "kibanaassociation.k8s.elastic.co/namespace"

	// KibanaSystemUserBuiltinRole is the name of the built-in role for the Kibana system user.
	KibanaSystemUserBuiltinRole = "kibana_system"
)

func AddKibanaES(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociationObjTemplate: func() commonv1.Association { return &kbv1.Kibana{} },
		ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
			return true, association.AssociationRef(), nil
		},
		ExternalServiceURL:  getElasticsearchExternalURL,
		AssociatedNamer:     esv1.ESNamer,
		AssociationName:     "kb-es",
		AssociatedShortName: "kb",
		AssociationLabels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				KibanaESAssociationLabelName:      associated.Name,
				KibanaESAssociationLabelNamespace: associated.Namespace,
			}
		},
		UserSecretSuffix:  "kibana-user",
		CASecretLabelName: eslabel.ClusterNameLabelName,
		ESUserRole: func(associated commonv1.Associated) (string, error) {
			return KibanaSystemUserBuiltinRole, nil
		},
	})
}
