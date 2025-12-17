// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	eprctl "github.com/elastic/cloud-on-k8s/v3/pkg/controller/packageregistry"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/rbac"
)

func AddKibanaEPR(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate:     func() commonv1.Associated { return &kbv1.Kibana{} },
		ReferencedObjTemplate:     func() client.Object { return &eprv1alpha1.PackageRegistry{} },
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
	epr := eprv1alpha1.PackageRegistry{}
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

// referencedEPRStatusVersion returns the currently running version of Package Registry
// reported in its status.
func referencedEPRStatusVersion(c k8s.Client, eprAssociation commonv1.Association) (string, bool, error) {
	eprRef := eprAssociation.AssociationRef()
	if eprRef.IsExternal() {
		// this should not happen (look at pkg/apis/kibana/v1/webhook.go)
		return "", false, errors.New("external Elastic Package Registry is not supported")
	}

	var epr eprv1alpha1.PackageRegistry
	err := c.Get(context.Background(), eprRef.NamespacedName(), &epr)
	if err != nil {
		return "", false, err
	}
	return epr.Status.Version, false, nil
}
