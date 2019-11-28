// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/name"
)

func Reconcile(
	d driver.Interface,
	kb kbv1.Kibana,
	services []corev1.Service,
	rotation certificates.RotationParams,
) *reconciler.Results {
	selfSignedCert := kb.Spec.HTTP.TLS.SelfSignedCertificate
	if selfSignedCert != nil && selfSignedCert.Disabled {
		return nil
	}
	results := reconciler.Results{}

	labels := label.NewLabels(kb.Name)

	// reconcile CA certs first
	httpCa, err := certificates.ReconcileCAForOwner(
		d.K8sClient(),
		d.Scheme(),
		name.KBNamer,
		&kb,
		labels,
		certificates.HTTPCAType,
		rotation,
	)
	if err != nil {
		return results.WithError(err)
	}

	// handle CA expiry via requeue
	results.WithResult(reconcile.Result{
		RequeueAfter: certificates.ShouldRotateIn(time.Now(), httpCa.Cert.NotAfter, rotation.RotateBefore),
	})

	// discover and maybe reconcile for the http certificates to use
	httpCertificates, err := http.ReconcileHTTPCertificates(
		d,
		&kb,
		name.KBNamer,
		httpCa,
		kb.Spec.HTTP.TLS,
		labels,
		services,
		rotation, // todo correct rotation
	)
	if err != nil {
		return results.WithError(err)
	}
	// reconcile http public cert secret
	results.WithError(http.ReconcileHTTPCertsPublicSecret(d.K8sClient(), d.Scheme(), &kb, name.KBNamer, httpCertificates))
	return &results
}
