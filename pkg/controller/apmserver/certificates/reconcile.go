// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"context"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/labels"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
)

func Reconcile(
	ctx context.Context,
	driver driver.Interface,
	as *apmv1.ApmServer,
	services []corev1.Service,
	caRotation certificates.RotationParams,
	certRotation certificates.RotationParams,
) *reconciler.Results {
	span, _ := apm.StartSpan(ctx, "reconcile_certs", tracing.SpanTypeApp)
	defer span.End()

	results := reconciler.NewResult(ctx)
	selfSignedCert := as.Spec.HTTP.TLS.SelfSignedCertificate
	if selfSignedCert != nil && selfSignedCert.Disabled {
		return results
	}

	labels := labels.NewLabels(as.Name)

	// reconcile CA certs first
	httpCa, err := certificates.ReconcileCAForOwner(
		driver.K8sClient(),
		driver.Scheme(),
		name.APMNamer,
		as,
		labels,
		certificates.HTTPCAType,
		caRotation,
	)
	if err != nil {
		return results.WithError(err)
	}

	// handle CA expiry via requeue
	results.WithResult(reconcile.Result{
		RequeueAfter: certificates.ShouldRotateIn(time.Now(), httpCa.Cert.NotAfter, caRotation.RotateBefore),
	})

	// discover and maybe reconcile for the http certificates to use
	httpCertificates, err := http.ReconcileHTTPCertificates(
		driver,
		as,
		name.APMNamer,
		httpCa,
		as.Spec.HTTP.TLS,
		labels,
		services,
		certRotation,
	)
	if err != nil {
		return results.WithError(err)
	}

	primaryCert, err := certificates.GetPrimaryCertificate(httpCertificates.CertPem())
	if err != nil {
		results.WithError(err)
	}
	results.WithResult(reconcile.Result{
		RequeueAfter: certificates.ShouldRotateIn(time.Now(), primaryCert.NotAfter, certRotation.RotateBefore),
	})

	// reconcile http public cert secret
	results.WithError(http.ReconcileHTTPCertsPublicSecret(driver.K8sClient(), driver.Scheme(), as, name.APMNamer, httpCertificates))
	return results
}
