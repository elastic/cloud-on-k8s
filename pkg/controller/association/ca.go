// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	eslabel "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// CASecret is a container to hold information about the Elasticsearch CA secret.
type CASecret struct {
	Name           string
	CACertProvided bool
}

// ServiceCaCertSecretName returns the name of the secret holding the certificate chain used
// by the associated resource to establish and validate a secured HTTP connection to the target service.
func ServiceCaCertSecretName(associated commonv1.Associated, associationName string) string {
	return associated.GetName() + "-" + associationName + "-ca"
}

// ReconcileCASecret keeps in sync a copy of the target service CA.
// It is the responsibility of the association controller to set a watch on this CA.
func (r *Reconciler) ReconcileCASecret(association commonv1.Association, namer name.Namer, service types.NamespacedName) (CASecret, error) {
	servicePublicHTTPCertificatesNSN := certificates.PublicCertsSecretRef(namer, service)

	// retrieve the HTTP certificates from ES namespace
	var servicePublicHTTPCertificatesSecret corev1.Secret
	if err := r.Get(servicePublicHTTPCertificatesNSN, &servicePublicHTTPCertificatesSecret); err != nil {
		if errors.IsNotFound(err) {
			return CASecret{}, nil // probably not created yet, we'll be notified to reconcile later
		}
		return CASecret{}, err
	}

	labels := r.AssociationLabels(k8s.ExtractNamespacedName(association))
	// Add the Elasticsearch name, this is only intended to help the user to filter on these resources
	labels[eslabel.ClusterNameLabelName] = service.Name

	// Certificate data should be copied over a secret in the association namespace
	expectedSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: association.GetNamespace(),
			Name:      ServiceCaCertSecretName(association, r.AssociationName),
			Labels:    labels,
		},
		Data: servicePublicHTTPCertificatesSecret.Data,
	}
	if _, err := reconciler.ReconcileSecret(r, expectedSecret, association.Associated()); err != nil {
		return CASecret{}, err
	}

	caCertProvided := len(expectedSecret.Data[certificates.CAFileName]) > 0
	return CASecret{Name: expectedSecret.Name, CACertProvided: caCertProvided}, nil
}
