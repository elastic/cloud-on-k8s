// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"context"
	"crypto/x509"
	"time"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/certificates/remoteca"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/certificates/transport"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// ReconcileHTTP reconciles the HTTP layer certificates of a cluster.
func ReconcileHTTP(
	ctx context.Context,
	driver driver.Interface,
	es esv1.Elasticsearch,
	services []corev1.Service,
	globalCA *certificates.CA,
	caRotation certificates.RotationParams,
	certRotation certificates.RotationParams,
) ([]*x509.Certificate, *reconciler.Results) {
	span, _ := apm.StartSpan(ctx, "reconcile_http_certs", tracing.SpanTypeApp)
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
		GlobalCA:       globalCA,
		CACertRotation: caRotation,
		CertRotation:   certRotation,
		// ES is able to hot-reload TLS certificates: let's keep secrets around even though TLS is disabled.
		// In case TLS is toggled on/off/on quickly enough, removing the secret would prevent future certs to be available.
		GarbageCollectSecrets: false,
	}.ReconcileCAAndHTTPCerts(ctx)
	if results.HasError() {
		_, err := results.Aggregate()
		k8s.MaybeEmitErrorEvent(driver.Recorder(), err, &es, events.EventReconciliationError, "Certificate reconciliation error: %v", err)
		return nil, results
	}

	trustedHTTPCertificates, err := certificates.ParsePEMCerts(httpCerts.CertChain())
	if err != nil {
		return nil, results.WithError(err)
	}

	return trustedHTTPCertificates, nil
}

// ReconcileTransport reconciles the transport layer certificates of a cluster.
func ReconcileTransport(
	ctx context.Context,
	driver driver.Interface,
	es esv1.Elasticsearch,
	globalCA *certificates.CA,
	caRotation certificates.RotationParams,
	certRotation certificates.RotationParams,
) *reconciler.Results {
	span, ctx := apm.StartSpan(ctx, "reconcile_transport_certs", tracing.SpanTypeApp)
	defer span.End()

	results := reconciler.NewResult(ctx)

	// label certificates secrets with the cluster name
	certsLabels := label.NewLabels(k8s.ExtractNamespacedName(&es))

	// Maybe retrieve user defined additional trusted CAs from a config map.
	// They will be concatenated with the ECK issued CA and distributed through the same transport secrets.
	additionalCAs, err := transport.ReconcileAdditionalCAs(ctx, driver.K8sClient(), es, driver.DynamicWatches())
	if err != nil {
		driver.Recorder().Eventf(&es, corev1.EventTypeWarning, events.EventReasonUnexpected, err.Error())
		return results.WithError(err)
	}

	// reconcile transport CA and certs
	transportCA, err := transport.ReconcileOrRetrieveCA(
		ctx,
		driver,
		es,
		certsLabels,
		globalCA,
		caRotation,
	)
	if err != nil {
		return results.WithError(err)
	}
	// make sure to requeue before the CA cert expires
	results.WithReconciliationState(
		reconciler.
			RequeueAfter(certificates.ShouldRotateIn(time.Now(), transportCA.Cert.NotAfter, caRotation.RotateBefore)).
			ReconciliationComplete(), // This reconciliation result should not prevent the reconciliation loop to be considered as completed in the status
	)

	// reconcile transport public certs secret
	if err := transport.ReconcileTransportCertsPublicSecret(ctx, driver.K8sClient(), es, transportCA, additionalCAs); err != nil {
		return results.WithError(err)
	}

	// reconcile transport certificates
	transportResults := transport.ReconcileTransportCertificatesSecrets(
		ctx,
		driver.K8sClient(),
		transportCA,
		additionalCAs,
		es,
		certRotation,
	)

	// reconcile remote clusters certificate authorities
	if err := remoteca.Reconcile(ctx, driver.K8sClient(), es, *transportCA); err != nil {
		results.WithError(err)
	}

	if results.WithResults(transportResults).HasError() {
		return results
	}

	return results
}
