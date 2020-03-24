// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// ReconcileTransportCertsPublicSecret reconciles the Secret containing the publicly available transport CA
// information.
func ReconcileTransportCertsPublicSecret(
	c k8s.Client,
	es esv1.Elasticsearch,
	ca *certificates.CA,
) error {
	esNSN := k8s.ExtractNamespacedName(&es)
	meta := k8s.ToObjectMeta(PublicCertsSecretRef(esNSN))
	meta.Labels = label.NewLabels(esNSN)

	expected := corev1.Secret{
		ObjectMeta: meta,
		Data: map[string][]byte{
			certificates.CAFileName: certificates.EncodePEMCert(ca.Cert.Raw),
		},
	}
	_, err := reconciler.ReconcileSecret(c, expected, &es)
	return err
}

// PublicCertsSecretRef returns the NamespacedName for the Secret containing the publicly available transport CA.
func PublicCertsSecretRef(es types.NamespacedName) types.NamespacedName {
	return types.NamespacedName{
		Name:      certificates.PublicCASecretName(esv1.ESNamer, es.Name),
		Namespace: es.Namespace,
	}
}
