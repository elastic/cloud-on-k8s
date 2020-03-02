// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"context"
	"crypto/x509"
	"time"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates/remoteca"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates/transport"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type CertificateResources struct {
	// TrustedHTTPCertificates contains the latest HTTP certificates that should be trusted.
	TrustedHTTPCertificates []*x509.Certificate

	// TransportCA is the CA used for Transport certificates
	TransportCA *certificates.CA

	// HTTPCACertProvided indicates whether ca.crt key is defined in the certificate secret.
	HTTPCACertProvided bool
}

// Reconcile reconciles the certificates of a cluster.
func Reconcile(
	ctx context.Context,
	driver driver.Interface,
	es esv1.Elasticsearch,
	services []corev1.Service,
	caRotation certificates.RotationParams,
	certRotation certificates.RotationParams,
) (*CertificateResources, *reconciler.Results) {
	span, _ := apm.StartSpan(ctx, "reconcile_certs", tracing.SpanTypeApp)
	defer span.End()

	results := &reconciler.Results{}

	// reconcile remote clusters certificate authorities
	if err := remoteca.Reconcile(driver.K8sClient(), es); err != nil {
		results.WithError(err)
	}

	labels := label.NewLabels(k8s.ExtractNamespacedName(&es))

	httpCA, err := certificates.ReconcileCAForOwner(
		driver.K8sClient(),
		driver.Scheme(),
		esv1.ESNamer,
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
		driver,
		&es,
		esv1.ESNamer,
		httpCA,
		es.Spec.HTTP.TLS,
		labels,
		services,
		certRotation,
	)
	if err != nil {
		return nil, results.WithError(err)
	}

	primaryCert, err := certificates.GetPrimaryCertificate(httpCertificates.CertPem())
	if err != nil {
		return nil, results.WithError(err)
	}
	results.WithResult(reconcile.Result{
		RequeueAfter: certificates.ShouldRotateIn(time.Now(), primaryCert.NotAfter, certRotation.RotateBefore),
	})

	// reconcile http public certs secret:
	if err := http.ReconcileHTTPCertsPublicSecret(driver.K8sClient(), driver.Scheme(), &es, esv1.ESNamer, httpCertificates); err != nil {
		return nil, results.WithError(err)
	}

	transportCA, err := certificates.ReconcileCAForOwner(
		driver.K8sClient(),
		driver.Scheme(),
		esv1.ESNamer,
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

	// reconcile transport public certs secret
	if err := transport.ReconcileTransportCertsPublicSecret(driver.K8sClient(), driver.Scheme(), es, transportCA); err != nil {
		return nil, results.WithError(err)
	}

	// reconcile transport certificates
	transportResults := transport.ReconcileTransportCertificatesSecrets(
		driver.K8sClient(),
		driver.Scheme(),
		transportCA,
		es,
		certRotation,
	)

	if results.WithResults(transportResults).HasError() {
		return nil, results
	}

	trustedHTTPCertificates, err := certificates.ParsePEMCerts(httpCertificates.CertPem())
	if err != nil {
		return nil, results.WithError(err)
	}

	httpCACertProvided := len(httpCertificates.Data[certificates.CAFileName]) > 0
	return &CertificateResources{
		TrustedHTTPCertificates: trustedHTTPCertificates,
		TransportCA:             transportCA,
		HTTPCACertProvided:      httpCACertProvided,
	}, results
}
