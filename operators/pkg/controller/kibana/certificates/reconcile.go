// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	coverv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Reconcile(
	c k8s.Client,
	scheme *runtime.Scheme,
	kb v1alpha1.Kibana,
	watches watches.DynamicWatches,
	services []coverv1.Service,
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
		c,
		scheme,
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
		c,
		scheme,
		watches,
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
	results.WithError(http.ReconcileHTTPCertsPublicSecret(c, scheme, &kb, name.KBNamer, httpCertificates))
	return &results
}
