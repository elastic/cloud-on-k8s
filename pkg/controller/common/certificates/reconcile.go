// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"context"
	"time"

	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	commonname "github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
)

var (
	log = ulog.Log.WithName("certificates")
)

type Reconciler struct {
	K8sClient      k8s.Client
	DynamicWatches watches.DynamicWatches

	Owner client.Object // owner for the TLS certificates (for ex. Elasticsearch, Kibana)

	TLSOptions    commonv1.TLSOptions               // TLS options of the object
	ExtraHTTPSANs []commonv1.SubjectAlternativeName // SANs dynamically set by a controller, only used in the self signed cert

	Namer    commonname.Namer  // helps naming the reconciled secrets
	Labels   map[string]string // to set on the reconciled cert secrets
	Services []corev1.Service  // to be used for TLS SANs

	CACertRotation RotationParams // to requeue a reconciliation before CA cert expiration
	CertRotation   RotationParams // to requeue a reconciliation before cert expiration

	GarbageCollectSecrets bool // if true, delete secrets if TLS is disabled
}

// ReconcileCAAndHTTPCerts reconciles 3 TLS-related secrets for the given object:
// - a Secret containing the Certificate Authority generated for the object
// - a Secret containing the HTTP certificates and key (for internal use by the object), returned by this function
// - a Secret containing the public-facing HTTP certificates (same as the internal one, but without the key)
// If TLS is disabled, self-signed certificates are still reconciled, for simplicity/consistency, but not used.
func (r Reconciler) ReconcileCAAndHTTPCerts(ctx context.Context) (*CertificatesSecret, *reconciler.Results) {
	span, _ := apm.StartSpan(ctx, "reconcile_certs", tracing.SpanTypeApp)
	defer span.End()

	results := reconciler.NewResult(ctx)

	if !r.TLSOptions.Enabled() && r.GarbageCollectSecrets {
		return nil, results.WithError(r.removeCAAndHTTPCertsSecrets())
	}

	// check for custom certificates first
	customCerts, err := validCustomCertificatesOrNil(r.K8sClient, k8s.ExtractNamespacedName(r.Owner), r.TLSOptions)
	if err != nil {
		return nil, results.WithError(err)
	}

	var httpCa *CA
	if customCerts.HasCAPrivateKey() {
		// if we have user-provided CA cert + key use that
		httpCa = customCerts.CA()
	} else {
		// if not then reconcile self-signed CA
		httpCa, err = ReconcileCAForOwner(
			r.K8sClient,
			r.Namer,
			r.Owner,
			r.Labels,
			HTTPCAType,
			r.CACertRotation,
		)
		if err != nil {
			return nil, results.WithError(err)
		}
		// handle CA expiry via requeue
		results.WithReconciliationState(
			reconciler.
				RequeueAfter(ShouldRotateIn(time.Now(), httpCa.Cert.NotAfter, r.CACertRotation.RotateBefore)).
				ReconciliationComplete(), // This reconciliation result should not prevent the reconciliation loop to be considered as completed in the status
		)
	}

	// reconcile http customCerts: either self-signed or user-provided
	httpCertificates, err := r.ReconcileInternalHTTPCerts(httpCa, customCerts)
	if err != nil {
		return nil, results.WithError(err)
	}
	primaryCert, err := GetPrimaryCertificate(httpCertificates.CertPem())
	if err != nil {
		return nil, results.WithError(err)
	}
	results.WithReconciliationState(
		reconciler.
			RequeueAfter(ShouldRotateIn(time.Now(), primaryCert.NotAfter, r.CertRotation.RotateBefore)).
			ReconciliationComplete(), // This reconciliation result should not prevent the reconciliation loop to be considered as completed in the status
	)

	// reconcile http public cert secret, which does not contain the private key
	results.WithError(r.ReconcilePublicHTTPCerts(httpCertificates))
	return httpCertificates, results
}

func (r *Reconciler) removeCAAndHTTPCertsSecrets() error {
	owner := k8s.ExtractNamespacedName(r.Owner)
	// remove public certs secret
	if err := k8s.DeleteSecretIfExists(r.K8sClient,
		types.NamespacedName{Namespace: owner.Namespace, Name: PublicCertsSecretName(r.Namer, owner.Name)},
	); err != nil {
		return err
	}
	// remove internal certs secret
	if err := k8s.DeleteSecretIfExists(r.K8sClient,
		types.NamespacedName{Namespace: owner.Namespace, Name: InternalCertsSecretName(r.Namer, owner.Name)},
	); err != nil {
		return err
	}
	// remove CA secret
	if err := k8s.DeleteSecretIfExists(r.K8sClient,
		types.NamespacedName{Namespace: owner.Namespace, Name: CAInternalSecretName(r.Namer, owner.Name, HTTPCAType)},
	); err != nil {
		return err
	}

	// remove watches on user-provided certs secret
	r.DynamicWatches.Secrets.RemoveHandlerForKey(CertificateWatchKey(r.Namer, r.Owner.GetName()))

	return nil
}
