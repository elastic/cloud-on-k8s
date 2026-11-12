// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"time"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/certificates/remoteca"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/certificates/transport"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
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
	meta metadata.Metadata,
) ([]*x509.Certificate, *reconciler.Results) {
	span, _ := apm.StartSpan(ctx, "reconcile_http_certs", tracing.SpanTypeApp)
	defer span.End()

	var results *reconciler.Results

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
		TLSOptions:     es.Spec.HTTP.TLS.TLSOptions,
		ExtraHTTPSANs:  extraHTTPSANs,
		Namer:          esv1.ESNamer,
		Metadata:       meta,
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
		k8s.MaybeEmitErrorEventf(driver.Recorder(), err, &es, events.EventReconciliationError, events.EventActionCertificateReconciliation, "Certificate reconciliation error: %v", err)
		return nil, results
	}

	trustedHTTPCertificates, err := certificates.ParsePEMCerts(httpCerts.CertChain())
	if err != nil {
		return nil, results.WithError(err)
	}

	return trustedHTTPCertificates, results
}

// ReconcileOperatorClientCertAndTrustBundle reconciles the operator client certificate and trust bundle of client
// certificates. It decides internally whether reconciliation is needed based on the clientAuthenticationRequired flag
// and the presence of the client-authentication-required annotation on the ES resource.
// When client authentication is enabled, the annotation is set to signal association controllers.
// Returns nil certificate (without error) when client authentication is not in use.
// Missing or invalid additional trusted CAs are left out of the trust bundle and returned as a separate error.
func ReconcileOperatorClientCertAndTrustBundle(
	ctx context.Context,
	driver driver.Interface,
	es *esv1.Elasticsearch,
	clientAuthenticationRequired bool,
	certRotation certificates.RotationParams,
	meta metadata.Metadata,
) (*tls.Certificate, *reconciler.Results, error) {
	span, ctx := apm.StartSpan(ctx, "reconcile_client_certs", tracing.SpanTypeApp)
	defer span.End()

	results := reconciler.NewResult(ctx)

	certReconciler := certificates.Reconciler{
		K8sClient:      driver.K8sClient(),
		DynamicWatches: driver.DynamicWatches(),
		Owner:          es,
		Namer:          esv1.ESNamer,
		Metadata:       meta,
		CertRotation:   certRotation,
	}

	// Client authentication is still in use while it is being disabled, until the annotation is removed.
	clientAuthenticationInUse := clientAuthenticationRequired || annotation.HasClientAuthenticationRequired(es)

	additionalTrustedCAs, additionalTrustedCAsReadErr := certReconciler.ReconcileAdditionalTrustedClientCAs(
		ctx, driver.Recorder(), es.Spec.HTTP.TLS.Client.AdditionalTrustedCAs, clientAuthenticationInUse)

	if !clientAuthenticationInUse {
		return nil, results, nil
	}

	if clientAuthenticationRequired {
		if err := annotation.SetClientAuthenticationRequiredAnnotation(ctx, driver.K8sClient(), es); err != nil {
			return nil, results.WithError(err), nil
		}
	}

	operatorClientCertSecret, err := certReconciler.ReconcileOperatorCertificate(ctx)
	if err != nil {
		return nil, results.WithError(err), nil
	}

	primaryCert, err := certificates.GetPrimaryCertificate(operatorClientCertSecret.CertPem())
	if err != nil {
		return nil, results.WithError(err), nil
	}
	results.WithReconciliationState(
		reconciler.
			RequeueAfter(certificates.ShouldRotateIn(time.Now(), primaryCert.NotAfter, certRotation.RotateBefore)).
			ReconciliationComplete(),
	)

	// Keep the trust bundle as it is if the additional CAs could not be read, not to drop CAs that are still trusted.
	if additionalTrustedCAsReadErr != nil {
		results.WithError(additionalTrustedCAsReadErr)
	} else if _, err := certReconciler.ReconcileTrustBundle(ctx, append([]*certificates.CertificatesSecret{operatorClientCertSecret}, additionalTrustedCAs.CAs...)...); err != nil {
		return nil, results.WithError(err), nil
	}
	var deferredErr error
	switch {
	case additionalTrustedCAs.InvalidErr == nil:
	case !clientAuthenticationRequired:
		// client authentication is being disabled, the CAs are about to be unused
		ulog.FromContext(ctx).V(1).Info("Ignoring the invalid additional trusted client CAs while client authentication is being disabled",
			"namespace", es.Namespace, "es_name", es.Name, "error", additionalTrustedCAs.InvalidErr.Error())
	default:
		// reported once the nodes are reconciled, not to block them
		deferredErr = additionalTrustedCAs.InvalidErr
	}

	operatorClientCert, err := certificates.ParseTLSCertificate(operatorClientCertSecret.Secret)
	if err != nil {
		return nil, results.WithError(err), deferredErr
	}

	return operatorClientCert, results, deferredErr
}

// ReconcileTransport reconciles the transport layer certificates of a cluster.
func ReconcileTransport(
	ctx context.Context,
	driver driver.Interface,
	es esv1.Elasticsearch,
	globalCA *certificates.CA,
	caRotation certificates.RotationParams,
	certRotation certificates.RotationParams,
	meta metadata.Metadata,
) *reconciler.Results {
	span, ctx := apm.StartSpan(ctx, "reconcile_transport_certs", tracing.SpanTypeApp)
	defer span.End()

	log := ulog.FromContext(ctx)
	results := reconciler.NewResult(ctx)

	// Maybe retrieve user defined additional trusted CAs from a config map.
	// They will be concatenated with the ECK issued CA and distributed through the same transport secrets.
	additionalCAs, err := transport.ReconcileAdditionalCAs(ctx, driver.K8sClient(), es, driver.DynamicWatches())
	if err != nil {
		k8s.EmitEvent(driver.Recorder(), &es, corev1.EventTypeWarning, events.EventReasonUnexpected, events.EventActionGetSecret, err.Error())
		return results.WithError(err)
	}

	// reconcile transport CA and certs
	transportCA, err := transport.ReconcileOrRetrieveCA(
		ctx,
		driver,
		es,
		meta,
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
	if err := transport.ReconcileTransportCertsPublicSecret(ctx, driver.K8sClient(), es, transportCA, additionalCAs, meta); err != nil {
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
		meta,
	)

	// reconcile remote clusters certificate authorities
	if err := remoteca.Reconcile(ctx, driver.K8sClient(), es, *transportCA, meta); err != nil {
		log.Error(err, "Failed to reconcile remote certificate authorities")
		k8s.MaybeEmitErrorEventf(driver.Recorder(), err, &es, events.EventReasonUnexpected,
			events.EventActionCertificateReconciliation, "Remote certificate authority reconciliation error: %v", err)
		results.WithReconciliationState(reconciler.RequeueAfter(reconciler.DefaultRequeue))
	}

	return results.WithResults(transportResults)
}
