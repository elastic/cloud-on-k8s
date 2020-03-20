// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package http

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ReconcileHTTPCertsPublicSecret reconciles the Secret containing the HTTP Certificate currently in use, and the CA of
// the certificate if available.
func ReconcileHTTPCertsPublicSecret(
	c k8s.Client,
	owner metav1.Object,
	namer name.Namer,
	httpCertificates *CertificatesSecret,
) error {
	expected := corev1.Secret{
		ObjectMeta: k8s.ToObjectMeta(PublicCertsSecretRef(namer, k8s.ExtractNamespacedName(owner))),
		Data: map[string][]byte{
			certificates.CertFileName: httpCertificates.CertPem(),
		},
	}
	if caPem := httpCertificates.CAPem(); caPem != nil {
		expected.Data[certificates.CAFileName] = caPem
	}

	_, err := reconciler.ReconcileSecret(c, expected, owner)
	return err
}

// PublicCertsSecretRef returns the NamespacedName for the Secret containing the publicly available HTTP CA.
func PublicCertsSecretRef(namer name.Namer, es types.NamespacedName) types.NamespacedName {
	return types.NamespacedName{
		Name:      certificates.PublicSecretName(namer, es.Name, certificates.HTTPCAType),
		Namespace: es.Namespace,
	}
}
