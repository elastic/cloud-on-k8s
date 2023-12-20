// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// CASecret is a container to hold information about the Elasticsearch CA secret.
type CASecret struct {
	Name           string
	CACertProvided bool
}

// CACertSecretName returns the name of the secret holding the certificate chain used
// by the associated resource to establish and validate a secured HTTP connection to the target service.
func CACertSecretName(association commonv1.Association, associationName string) string {
	associatedName := association.Associated().GetName()
	return commonv1.FormatNameWithID(associatedName+"-"+associationName+"%s-ca", association.AssociationID())
}

// ReconcileCASecret keeps in sync a copy of the target service CA.
// It is the responsibility of the association controller to set a watch on this CA.
func (r *Reconciler) ReconcileCASecret(ctx context.Context, association commonv1.Association, namer name.Namer, associatedResource types.NamespacedName) (CASecret, error) {
	associatedPublicHTTPCertificatesNSN := certificates.PublicCertsSecretRef(namer, associatedResource)

	// retrieve the HTTP certificates from the associatedResource namespace
	var associatedPublicHTTPCertificatesSecret corev1.Secret
	if err := r.Get(ctx, associatedPublicHTTPCertificatesNSN, &associatedPublicHTTPCertificatesSecret); err != nil {
		if errors.IsNotFound(err) {
			return CASecret{}, nil // probably not created yet, we'll be notified to reconcile later
		}
		return CASecret{}, err
	}

	labels := r.AssociationResourceLabels(k8s.ExtractNamespacedName(association), association.AssociationRef().NamespacedName())
	// Certificate data should be copied over a secret in the association namespace
	expectedSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: association.GetNamespace(),
			Name:      CACertSecretName(association, r.AssociationName),
			Labels:    labels,
		},
		Data: associatedPublicHTTPCertificatesSecret.Data,
	}
	if _, err := reconciler.ReconcileSecret(ctx, r, expectedSecret, association.Associated()); err != nil {
		return CASecret{}, err
	}

	caCertProvided := len(expectedSecret.Data[certificates.CAFileName]) > 0
	return CASecret{Name: expectedSecret.Name, CACertProvided: caCertProvided}, nil
}
