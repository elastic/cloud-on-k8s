// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package http

import (
	"reflect"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// ReconcileHTTPCertsPublicSecret reconciles the Secret containing the HTTP Certificate currently in use, and the CA of
// the certificate if available.
func ReconcileHTTPCertsPublicSecret(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
	httpCertificates *CertificatesSecret,
) error {
	expected := &corev1.Secret{
		ObjectMeta: k8s.ToObjectMeta(PublicCertsSecretRef(k8s.ExtractNamespacedName(&es))),
		Data: map[string][]byte{
			certificates.CertFileName: httpCertificates.CertPem(),
		},
	}

	// TODO: reconcile labels and annotations?

	reconciled := &corev1.Secret{}

	return reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     scheme,
		Owner:      &es,
		Expected:   expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			// TODO: these label and annotation comparisons are very strict
			if !reflect.DeepEqual(reconciled.Labels, expected.Labels) {
				return true
			}
			if !reflect.DeepEqual(reconciled.Annotations, expected.Annotations) {
				return true
			}
			if !reflect.DeepEqual(reconciled.Data, expected.Data) {
				return true
			}
			return false
		},
		UpdateReconciled: func() {
			reconciled.Labels = expected.Labels
			reconciled.Annotations = expected.Annotations
			reconciled.Data = expected.Data
		},
	})
}

// PublicCertsSecretRef returns the NamespacedName for the Secret containing the publicly available HTTP CA.
func PublicCertsSecretRef(es types.NamespacedName) types.NamespacedName {
	return types.NamespacedName{
		Name:      name.CertsPublicSecretName(es.Name, certificates.HTTPCAType),
		Namespace: es.Namespace,
	}
}
