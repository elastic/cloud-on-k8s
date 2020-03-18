// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/ca"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/certutils"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
)

// ReconcileTransportCertsPublicSecret reconciles the Secret containing the publicly available transport CA
// information.
func ReconcileTransportCertsPublicSecret(
	c k8s.Client,
	es esv1.Elasticsearch,
	ca *ca.CA,
) error {
	esNSN := k8s.ExtractNamespacedName(&es)
	meta := k8s.ToObjectMeta(PublicCertsSecretRef(esNSN))
	meta.Labels = label.NewLabels(esNSN)

	expected := &corev1.Secret{
		ObjectMeta: meta,
		Data: map[string][]byte{
			certutils.CAFileName: certutils.EncodePEMCert(ca.Cert.Raw),
		},
	}
	reconciled := &corev1.Secret{}

	return reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Owner:      &es,
		Expected:   expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			switch {
			case !maps.IsSubset(expected.Labels, reconciled.Labels):
				return true
			case !maps.IsSubset(expected.Annotations, reconciled.Annotations):
				return true
			case !reflect.DeepEqual(expected.Data, reconciled.Data):
				return true
			default:
				return false
			}
		},
		UpdateReconciled: func() {
			reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
			reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
			reconciled.Data = expected.Data
		},
	})
}

// PublicCertsSecretRef returns the NamespacedName for the Secret containing the publicly available transport CA.
func PublicCertsSecretRef(es types.NamespacedName) types.NamespacedName {
	return types.NamespacedName{
		Name:      esv1.ESNamer.Suffix(es.Name, "transport", "certs-public"),
		Namespace: es.Namespace,
	}
}
