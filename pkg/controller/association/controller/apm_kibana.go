// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user"
	kblabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/rbac"
)

func AddApmKibana(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedShortName:       "apm",
		AssociatedObjTemplate:     func() commonv1.Associated { return &apmv1.ApmServer{} },
		ReferencedObjTemplate:     func() client.Object { return &kbv1.Kibana{} },
		ExternalServiceURL:        getKibanaExternalURL,
		ReferencedResourceVersion: referencedKibanaStatusVersion,
		ReferencedResourceNamer:   kbv1.KBNamer,
		AssociationName:           "apm-kibana",
		AssociationType:           commonv1.KibanaAssociationType,
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				ApmAssociationLabelName:      associated.Name,
				ApmAssociationLabelNamespace: associated.Namespace,
				ApmAssociationLabelType:      commonv1.KibanaAssociationType,
			}
		},
		AssociationConfAnnotationNameBase:     commonv1.KibanaConfigAnnotationNameBase,
		AssociationResourceNameLabelName:      kblabel.KibanaNameLabelName,
		AssociationResourceNamespaceLabelName: kblabel.KibanaNamespaceLabelName,

		ElasticsearchUserCreation: &association.ElasticsearchUserCreation{
			ElasticsearchRef: getElasticsearchFromKibana,
			UserSecretSuffix: "apm-kb-user",
			ESUserRole: func(_ commonv1.Associated) (string, error) {
				return user.ApmAgentUserRole, nil
			},
		},
	})
}

func getKibanaExternalURL(c k8s.Client, assoc commonv1.Association) (string, error) {
	kibanaRef := assoc.AssociationRef()
	if !kibanaRef.IsDefined() {
		return "", nil
	}
	kb := kbv1.Kibana{}
	if err := c.Get(context.Background(), kibanaRef.NamespacedName(), &kb); err != nil {
		return "", err
	}
	serviceName := kibanaRef.ServiceName
	if serviceName == "" {
		serviceName = kbv1.HTTPService(kb.Name)
	}
	nsn := types.NamespacedName{Namespace: kb.Namespace, Name: serviceName}
	return association.ServiceURL(c, nsn, kb.Spec.HTTP.Protocol())
}

// referencedKibanaStatusVersion returns the currently running version of Kibana
// reported in its status.
func referencedKibanaStatusVersion(c k8s.Client, kbAssociation commonv1.Association) (string, error) {
	kbRef := kbAssociation.AssociationRef()
	if kbRef.IsExternal() {
		info, err := association.GetUnmanagedAssociationConnectionInfoFromSecret(c, kbAssociation)
		if err != nil {
			return "", err
		}
		ver, err := info.Version("/api/status", "{ .version.number }")
		if err != nil {
			return "", err
		}
		return ver, nil
	}

	var kb kbv1.Kibana
	err := c.Get(context.Background(), kbRef.NamespacedName(), &kb)
	if err != nil {
		return "", err
	}
	return kb.Status.Version, nil
}

// getElasticsearchFromKibana returns the Elasticsearch reference in which the user must be created for this association.
func getElasticsearchFromKibana(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
	kibanaRef := association.AssociationRef()
	if !kibanaRef.IsDefined() {
		return false, commonv1.ObjectSelector{}, nil
	}

	kb := kbv1.Kibana{}
	err := c.Get(context.Background(), kibanaRef.NamespacedName(), &kb)
	if errors.IsNotFound(err) {
		return false, commonv1.ObjectSelector{}, nil
	}
	if err != nil {
		return false, commonv1.ObjectSelector{}, err
	}

	return true, kb.EsAssociation().AssociationRef(), nil
}
