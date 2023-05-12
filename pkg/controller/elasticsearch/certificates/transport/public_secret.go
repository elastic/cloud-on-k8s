// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package transport

import (
	"bytes"
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// ReconcileTransportCertsPublicSecret reconciles the Secret containing the publicly available transport CA
// information.
func ReconcileTransportCertsPublicSecret(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	ca *certificates.CA,
	additionalCAs []byte,
) error {
	esNSN := k8s.ExtractNamespacedName(&es)
	meta := k8s.ToObjectMeta(PublicCertsSecretRef(esNSN))
	meta.Labels = label.NewLabels(esNSN)

	expected := corev1.Secret{
		ObjectMeta: meta,
		Data: map[string][]byte{
			certificates.CAFileName: bytes.Join([][]byte{certificates.EncodePEMCert(ca.Cert.Raw), additionalCAs}, nil),
		},
	}

	// Don't set an ownerRef for public transport certs secrets, likely to be copied into different namespaces.
	// See https://github.com/elastic/cloud-on-k8s/issues/3986.
	_, err := reconciler.ReconcileSecretNoOwnerRef(ctx, c, expected, &es)
	return err
}

// PublicCertsSecretRef returns the NamespacedName for the Secret containing the publicly available transport CA.
func PublicCertsSecretRef(es types.NamespacedName) types.NamespacedName {
	return types.NamespacedName{
		Name:      certificates.PublicTransportCertsSecretName(esv1.ESNamer, es.Name),
		Namespace: es.Namespace,
	}
}
