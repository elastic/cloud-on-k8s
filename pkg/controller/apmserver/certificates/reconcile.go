// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/labels"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	coverv1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Reconcile(
	driver driver.Interface,
	apm *v1alpha1.ApmServer,
	services []coverv1.Service,
	rotation certificates.RotationParams,
) reconciler.Results {
	results := reconciler.Results{}
	selfSignedCert := apm.Spec.HTTP.TLS.SelfSignedCertificate
	if selfSignedCert != nil && selfSignedCert.Disabled {
		return results
	}

	labels := labels.NewLabels(apm.Name)

	// reconcile CA certs first
	httpCa, err := certificates.ReconcileCAForOwner(
		driver.K8sClient(),
		driver.Scheme(),
		name.APMNamer,
		apm,
		labels,
		certificates.HTTPCAType,
		rotation,
	)
	if err != nil {
		return *results.WithError(err)
	}

	// handle CA expiry via requeue
	results.WithResult(reconcile.Result{
		RequeueAfter: certificates.ShouldRotateIn(time.Now(), httpCa.Cert.NotAfter, rotation.RotateBefore),
	})

	// discover and maybe reconcile for the http certificates to use
	httpCertificates, err := http.ReconcileHTTPCertificates(
		driver,
		apm,
		name.APMNamer,
		httpCa,
		apm.Spec.HTTP.TLS,
		labels,
		services,
		rotation, // todo correct rotation
	)
	if err != nil {
		return *results.WithError(err)
	}
	// reconcile http public cert secret
	results.WithError(http.ReconcileHTTPCertsPublicSecret(driver.K8sClient(), driver.Scheme(), apm, name.APMNamer, httpCertificates))
	return results
}
