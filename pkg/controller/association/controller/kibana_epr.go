// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/epr/v1alpha1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	ver "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	eprctl "github.com/elastic/cloud-on-k8s/v3/pkg/controller/packageregistry"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/rbac"
)

func AddKibanaEPR(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate:     func() commonv1.Associated { return &kbv1.Kibana{} },
		ReferencedObjTemplate:     func() client.Object { return &eprv1alpha1.ElasticPackageRegistry{} },
		ExternalServiceURL:        getEPRExternalURL,
		ReferencedResourceVersion: referencedEPRStatusVersion,
		ReferencedResourceNamer:   eprv1alpha1.Namer,
		AssociationName:           "kb-epr",
		AssociatedShortName:       "kb",
		AssociationType:           commonv1.PackageRegistryAssociationType,
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				KibanaAssociationLabelName:      associated.Name,
				KibanaAssociationLabelNamespace: associated.Namespace,
				KibanaAssociationLabelType:      commonv1.PackageRegistryAssociationType,
			}
		},
		AssociationConfAnnotationNameBase:     commonv1.EPRConfigAnnotationNameBase,
		AssociationResourceNameLabelName:      eprctl.NameLabelName,
		AssociationResourceNamespaceLabelName: eprctl.PackageRegistryNamespaceLabelName,
		ElasticsearchUserCreation:             nil, // no dedicated ES user required for Kibana->EPR connection
	})
}

func getEPRExternalURL(c k8s.Client, assoc commonv1.Association) (string, error) {
	eprRef := assoc.AssociationRef()
	if !eprRef.IsDefined() {
		return "", nil
	}
	epr := eprv1alpha1.ElasticPackageRegistry{}
	if err := c.Get(context.Background(), eprRef.NamespacedName(), &epr); err != nil {
		return "", err
	}
	serviceName := eprRef.ServiceName
	if serviceName == "" {
		serviceName = eprctl.HTTPServiceName(epr.Name)
	}
	nsn := types.NamespacedName{Namespace: epr.Namespace, Name: serviceName}
	return association.ServiceURL(c, nsn, epr.Spec.HTTP.Protocol(), "")
}

type eprVersionResponse struct {
	Number string `json:"number"`
}

func (eprVersionResponse) IsServerless() bool {
	return false
}

func (evr eprVersionResponse) GetVersion() (string, error) {
	if _, err := ver.Parse(evr.Number); err != nil {
		return "", err
	}
	return evr.Number, nil
}

// referencedEPRStatusVersion returns the currently running version of Package Registry
// reported in its status.
func referencedEPRStatusVersion(c k8s.Client, eprAssociation commonv1.Association) (string, bool, error) {
	eprRef := eprAssociation.AssociationRef()
	if eprRef.IsExternal() {
		info, err := association.GetUnmanagedAssociationConnectionInfoFromSecret(c, eprAssociation)
		if err != nil {
			return "", false, err
		}
		eprVersionResponse := &eprVersionResponse{}
		ver, isServerless, err := info.Version("/api/epr/v1/internal/version", eprVersionResponse)
		if err != nil {
			return "", false, err
		}
		return ver, isServerless, nil
	}

	var epr eprv1alpha1.ElasticPackageRegistry
	err := c.Get(context.Background(), eprRef.NamespacedName(), &epr)
	if err != nil {
		return "", false, err
	}
	return epr.Status.Version, false, nil
}
