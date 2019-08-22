// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"crypto/x509"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/certificates/transport"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type CertificateResources struct {
	// TrustedHTTPCertificates contains the latest HTTP certificates that should be trusted.
	TrustedHTTPCertificates []*x509.Certificate

	// TransportCA is the CA used for Transport certificates
	TransportCA *certificates.CA
}

// reconcileGenericResources reconciles the expected generic resources of a cluster.
func Reconcile(
	c k8s.Client,
	scheme *runtime.Scheme,
	watches watches.DynamicWatches,
	es v1alpha1.Elasticsearch,
	services []corev1.Service,
	caRotation certificates.RotationParams,
	certRotation certificates.RotationParams,
) (*CertificateResources, *reconciler.Results) {
	results := &reconciler.Results{}

	labels := label.NewLabels(k8s.ExtractNamespacedName(&es))

	httpCA, err := certificates.ReconcileCAForOwner(
		c,
		scheme,
		name.ESNamer,
		&es,
		labels,
		certificates.HTTPCAType,
		caRotation,
	)
	if err != nil {
		return nil, results.WithError(err)
	}

	// make sure to requeue before the CA cert expires
	results.WithResult(reconcile.Result{
		RequeueAfter: certificates.ShouldRotateIn(time.Now(), httpCA.Cert.NotAfter, caRotation.RotateBefore),
	})

	// discover and maybe reconcile for the http certificates to use
	httpCertificates, err := http.ReconcileHTTPCertificates(
		c,
		scheme,
		watches,
		&es,
		name.ESNamer,
		httpCA,
		es.Spec.HTTP.TLS,
		labels,
		services,
		caRotation,
	)
	if err != nil {
		return nil, results.WithError(err)
	}

	// reconcile http public certs secret:
	if err := http.ReconcileHTTPCertsPublicSecret(c, scheme, &es, name.ESNamer, httpCertificates); err != nil {
		return nil, results.WithError(err)
	}

	transportCA, err := certificates.ReconcileCAForOwner(
		c,
		scheme,
		name.ESNamer,
		&es,
		labels,
		certificates.TransportCAType,
		caRotation,
	)
	if err != nil {
		return nil, results.WithError(err)
	}
	// make sure to requeue before the CA cert expires
	results.WithResult(reconcile.Result{
		RequeueAfter: certificates.ShouldRotateIn(time.Now(), transportCA.Cert.NotAfter, caRotation.RotateBefore),
	})

	// reconcile transport public certs secret:
	if err := transport.ReconcileTransportCertsPublicSecret(c, scheme, es, transportCA); err != nil {
		return nil, results.WithError(err)
	}

	// reconcile transport certificates
	result, err := transport.ReconcileTransportCertificatesSecrets(
		c,
		scheme,
		transportCA,
		es,
		certRotation,
	)
	if results.WithResult(result).WithError(err).HasError() {
		return nil, results
	}

	trustedHTTPCertificates, err := certificates.ParsePEMCerts(httpCertificates.CertPem())
	if err != nil {
		return nil, results.WithError(err)
	}

	return &CertificateResources{
		TrustedHTTPCertificates: trustedHTTPCertificates,
		TransportCA:             transportCA,
	}, results
}
