// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"context"
	"time"

	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/ca"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/certutils"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/http"
	commonname "github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// ReconcileCAAndHTTPCerts reconciles 3 TLS-related secrets for the given object:
// - a Secret containing the Certificate Authority generated for the object
// - a Secret containing the HTTP certificates and key (for internal use by the object), returned by this function
// - a Secret containing the public-facing HTTP certificates (same as the internal one, but without the key)
// If TLS is disabled, self-signed certificates are still reconciled, for simplicity/consistency, but not used.
func ReconcileCAAndHTTPCerts(
	ctx context.Context,
	object metav1.Object,
	tlsOptions commonv1.TLSOptions,
	labels map[string]string,
	namer commonname.Namer,
	k8sClient k8s.Client,
	dynamicWatches watches.DynamicWatches,
	services []corev1.Service,
	caRotation certutils.RotationParams,
	certRotation certutils.RotationParams,
) (*http.CertificatesSecret, *reconciler.Results) {
	span, _ := apm.StartSpan(ctx, "reconcile_certs", tracing.SpanTypeApp)
	defer span.End()

	results := reconciler.NewResult(ctx)

	// reconcile CA certs first
	httpCa, err := ca.ReconcileCAForOwner(
		k8sClient,
		namer,
		object,
		labels,
		ca.HTTPCAType,
		caRotation,
	)
	if err != nil {
		return nil, results.WithError(err)
	}
	// handle CA expiry via requeue
	results.WithResult(reconcile.Result{
		RequeueAfter: certutils.ShouldRotateIn(time.Now(), httpCa.Cert.NotAfter, caRotation.RotateBefore),
	})

	// reconcile http certificates: either self-signed or user-provided
	httpCertificates, err := http.ReconcileHTTPCertificates(
		k8sClient,
		dynamicWatches,
		object,
		namer,
		httpCa,
		tlsOptions,
		labels,
		services,
		certRotation,
	)
	if err != nil {
		return nil, results.WithError(err)
	}
	primaryCert, err := certutils.GetPrimaryCertificate(httpCertificates.CertPem())
	if err != nil {
		return nil, results.WithError(err)
	}
	results.WithResult(reconcile.Result{
		RequeueAfter: certutils.ShouldRotateIn(time.Now(), primaryCert.NotAfter, certRotation.RotateBefore),
	})

	// reconcile http public cert secret, which does not contain the private key
	results.WithError(http.ReconcileHTTPCertsPublicSecret(k8sClient, object, namer, httpCertificates))
	return httpCertificates, results
}
