// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"context"
	"time"

	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	commonname "github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var (
	log = logf.Log.WithName("certificates")
)

type Reconciler struct {
	K8sClient      k8s.Client
	DynamicWatches watches.DynamicWatches

	Object     metav1.Object       // owner for the TLS certificates (eg. Elasticsearch, Kibana)
	TLSOptions commonv1.TLSOptions // TLS options of the object

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

	// reconcile CA certs first
	httpCa, err := ReconcileCAForOwner(
		r.K8sClient,
		r.Namer,
		r.Object,
		r.Labels,
		HTTPCAType,
		r.CACertRotation,
	)
	if err != nil {
		return nil, results.WithError(err)
	}
	// handle CA expiry via requeue
	results.WithResult(reconcile.Result{
		RequeueAfter: ShouldRotateIn(time.Now(), httpCa.Cert.NotAfter, r.CACertRotation.RotateBefore),
	})

	// reconcile http certificates: either self-signed or user-provided
	httpCertificates, err := r.ReconcileInternalHTTPCerts(httpCa)
	if err != nil {
		return nil, results.WithError(err)
	}
	primaryCert, err := GetPrimaryCertificate(httpCertificates.CertPem())
	if err != nil {
		return nil, results.WithError(err)
	}
	results.WithResult(reconcile.Result{
		RequeueAfter: ShouldRotateIn(time.Now(), primaryCert.NotAfter, r.CertRotation.RotateBefore),
	})

	// reconcile http public cert secret, which does not contain the private key
	results.WithError(r.ReconcilePublicHTTPCerts(httpCertificates))
	return httpCertificates, results
}

func (r *Reconciler) removeCAAndHTTPCertsSecrets() error {
	// remove public certs secret
	if err := deleteIfExists(r.K8sClient,
		types.NamespacedName{Namespace: r.Object.GetNamespace(), Name: PublicCertsSecretName(r.Namer, r.Object.GetName())},
	); err != nil {
		return err
	}
	// remove internal certs secret
	if err := deleteIfExists(r.K8sClient,
		types.NamespacedName{Namespace: r.Object.GetNamespace(), Name: InternalCertsSecretName(r.Namer, r.Object.GetName())},
	); err != nil {
		return err
	}
	// remove CA secret
	if err := deleteIfExists(r.K8sClient,
		types.NamespacedName{Namespace: r.Object.GetNamespace(), Name: CAInternalSecretName(r.Namer, r.Object.GetName(), HTTPCAType)},
	); err != nil {
		return err
	}

	// remove watches on user-provided certs secret
	r.DynamicWatches.Secrets.RemoveHandlerForKey(CertificateWatchKey(r.Namer, r.Object.GetName()))

	return nil
}

func deleteIfExists(c k8s.Client, secretRef types.NamespacedName) error {
	var secret corev1.Secret
	err := c.Get(secretRef, &secret)
	if err != nil && apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}
	log.Info("Deleting secret", "namespace", secretRef.Namespace, "secret_name", secretRef.Name)
	err = c.Delete(&secret)
	if err != nil && apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
