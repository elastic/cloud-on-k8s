// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	ver "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	entctl "github.com/elastic/cloud-on-k8s/v2/pkg/controller/enterprisesearch"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/rbac"
)

func AddKibanaEnt(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate:     func() commonv1.Associated { return &kbv1.Kibana{} },
		ReferencedObjTemplate:     func() client.Object { return &entv1.EnterpriseSearch{} },
		ExternalServiceURL:        getEntExternalURL,
		ReferencedResourceVersion: referencedEntStatusVersion,
		ReferencedResourceNamer:   entv1.Namer,
		AssociationName:           "kb-ent",
		AssociatedShortName:       "kb",
		AssociationType:           commonv1.EntAssociationType,
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				KibanaAssociationLabelName:      associated.Name,
				KibanaAssociationLabelNamespace: associated.Namespace,
				KibanaAssociationLabelType:      commonv1.EntAssociationType,
			}
		},
		AssociationConfAnnotationNameBase:     commonv1.EntConfigAnnotationNameBase,
		AssociationResourceNameLabelName:      entctl.EnterpriseSearchNameLabelName,
		AssociationResourceNamespaceLabelName: entctl.EnterpriseSearchNamespaceLabelName,
		ElasticsearchUserCreation:             nil, // no dedicated ES user required for Kibana->Ent connection
	})
}

func getEntExternalURL(c k8s.Client, assoc commonv1.Association) (string, error) {
	entRef := assoc.AssociationRef()
	if !entRef.IsDefined() {
		return "", nil
	}
	ent := entv1.EnterpriseSearch{}
	if err := c.Get(context.Background(), entRef.NamespacedName(), &ent); err != nil {
		return "", err
	}
	serviceName := entRef.ServiceName
	if serviceName == "" {
		serviceName = entctl.HTTPServiceName(ent.Name)
	}
	nsn := types.NamespacedName{Namespace: ent.Namespace, Name: serviceName}
	return association.ServiceURL(c, nsn, ent.Spec.HTTP.Protocol())
}

type entVersionResponse struct {
	Number string `json:"number"`
}

func (entVersionResponse) IsServerless() bool {
	return false
}

func (evr entVersionResponse) GetVersion() (string, error) {
	if _, err := ver.Parse(evr.Number); err != nil {
		return "", err
	}
	return evr.Number, nil
}

// referencedEntStatusVersion returns the currently running version of Enterprise Search
// reported in its status.
func referencedEntStatusVersion(c k8s.Client, entAssociation commonv1.Association) (string, bool, error) {
	entRef := entAssociation.AssociationRef()
	if entRef.IsExternal() {
		info, err := association.GetUnmanagedAssociationConnectionInfoFromSecret(c, entAssociation)
		if err != nil {
			return "", false, err
		}
		entVersionResponse := &entVersionResponse{}
		ver, isServerless, err := info.Version("/api/ent/v1/internal/version", entVersionResponse)
		if err != nil {
			return "", false, err
		}
		return ver, isServerless, nil
	}

	var ent entv1.EnterpriseSearch
	err := c.Get(context.Background(), entRef.NamespacedName(), &ent)
	if err != nil {
		return "", false, err
	}
	return ent.Status.Version, false, nil
}
