// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"context"
	"crypto/x509"
	"time"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates/remoteca"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates/transport"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
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

	var results *reconciler.Results

	// label certificates secrets with the cluster name
	certsLabels := label.NewLabels(k8s.ExtractNamespacedName(&es))

	// Create some additional SANs, mostly to be used in the context of client autodiscovery (a.k.a. sniffing).
	extraHTTPSANs := make([]commonv1.SubjectAlternativeName, len(es.Spec.NodeSets))
	for i, nodeSet := range es.Spec.NodeSets {
		extraHTTPSANs[i] =
			commonv1.SubjectAlternativeName{DNS: "*." + nodespec.HeadlessServiceName(esv1.StatefulSet(es.Name, nodeSet.Name)) + "." + es.Namespace + ".svc"}
	}

	// reconcile HTTP CA and cert
	var httpCerts *certificates.CertificatesSecret
	httpCerts, results = certificates.Reconciler{
		K8sClient:      driver.K8sClient(),
		DynamicWatches: driver.DynamicWatches(),
		Owner:          &es,
		TLSOptions:     es.Spec.HTTP.TLS,
		ExtraHTTPSANs:  extraHTTPSANs,
		Namer:          esv1.ESNamer,
		Labels:         certsLabels,
		Services:       services,
		CACertRotation: caRotation,
		CertRotation:   certRotation,
		// ES is able to hot-reload TLS certificates: let's keep secrets around even though TLS is disabled.
		// In case TLS is toggled on/off/on quickly enough, removing the secret would prevent future certs to be available.
		GarbageCollectSecrets: false,
	}.ReconcileCAAndHTTPCerts(ctx)
	if results.HasError() {
		return nil, results
	}

	// reconcile transport CA and certs
	transportCA, err := transport.ReconcileOrRetrieveCA(
		driver,
		es,
		certsLabels,
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
	if err := transport.ReconcileTransportCertsPublicSecret(driver.K8sClient(), es, transportCA); err != nil {
		return nil, results.WithError(err)
	}

	// reconcile transport certificates
	transportResults := transport.ReconcileTransportCertificatesSecrets(
		driver.K8sClient(),
		transportCA,
		es,
		certRotation,
	)

	// reconcile remote clusters certificate authorities
	if err := remoteca.Reconcile(driver.K8sClient(), es, *transportCA); err != nil {
		results.WithError(err)
	}

	if results.WithResults(transportResults).HasError() {
		return nil, results
	}

	trustedHTTPCertificates, err := certificates.ParsePEMCerts(httpCerts.CertPem())
	if err != nil {
		return nil, results.WithError(err)
	}

	return &CertificateResources{
		TrustedHTTPCertificates: trustedHTTPCertificates,
		TransportCA:             transportCA,
	}, results
}
