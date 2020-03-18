// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package http

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/certutils"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
)

// ReconcileHTTPCertsPublicSecret reconciles the Secret containing the HTTP Certificate currently in use, and the CA of
// the certificate if available.
func ReconcileHTTPCertsPublicSecret(
	c k8s.Client,
	owner metav1.Object,
	namer name.Namer,
	httpCertificates *CertificatesSecret,
	labels map[string]string,
) error {
	nsn := PublicCertsSecretRef(namer, k8s.ExtractNamespacedName(owner))
	expected := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: nsn.Namespace,
			Name:      nsn.Name,
			Labels:    labels,
		},
		Data: map[string][]byte{
			certutils.CertFileName: httpCertificates.CertPem(),
		},
	}
	if caPem := httpCertificates.CAPem(); caPem != nil {
		expected.Data[certutils.CAFileName] = caPem
	}

	reconciled := &corev1.Secret{}

	return reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Owner:      owner,
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

// PublicCertsSecretRef returns the NamespacedName for the Secret containing the publicly available HTTP CA.
func PublicCertsSecretRef(namer name.Namer, es types.NamespacedName) types.NamespacedName {
	return types.NamespacedName{
		Name:      PublicCertsSecretName(namer, es.Name),
		Namespace: es.Namespace,
	}
}
