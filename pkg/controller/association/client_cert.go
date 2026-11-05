// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"crypto/tls"
	"fmt"
	"maps"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// filterWithUserProvidedClientCert returns associations that have a user-provided client certificate secret.
func filterWithUserProvidedClientCert(associations []commonv1.Association) []commonv1.Association {
	var r []commonv1.Association
	for _, a := range associations {
		if a.AssociationRef().GetClientCertificateSecretName() != "" {
			r = append(r, a)
		}
	}
	return r
}

// ClientCertNamer is a Namer for client certificate secrets, which can have longer names than
// label-constrained resources since secrets use DNS subdomain naming (253 chars max).
var ClientCertNamer = name.NewSecretNamer()

// clientCertSecretName returns the name of the client certificate secret for an association.
// The name is collision-free: it includes a hash of the referenced (server) resource's namespace and name.
func clientCertSecretName(association commonv1.Association, associationName string) string {
	associatedName := association.Associated().GetName()
	ref := association.AssociationRef()
	refHash := hash.HashObject(ref.NamespacedName())
	return ClientCertNamer.Suffix(associatedName, associationName, refHash, "client-cert")
}

// reconcileClientCertificate reconciles a client certificate for an association when the referenced
// resource requires client authentication. If the user has specified a client certificate secret via
// ClientCertificateSecretName on the association ref, its contents are copied into the ECK-managed secret. Otherwise,
// a self-signed client certificate is auto-generated.
//
// The ECK-managed secret is created in the associated resource's namespace so the client can mount it. Its
// public certificate is also published as a trust source into the referenced server's namespace.
//
// Returns the client certificate secret and results containing requeue and/or error info.
func (r *Reconciler) reconcileClientCertificate(
	ctx context.Context,
	association commonv1.Association,
	referencedObj client.Object,
	assocMeta metadata.Metadata,
) (*certificates.CertificatesSecret, *reconciler.Results) {
	results := reconciler.NewResult(ctx)

	secretName := clientCertSecretName(association, r.AssociationName)
	ref := association.AssociationRef()

	extraLabels := map[string]string{
		labels.ClientCertificateLabelName: "true",
	}

	var clientCertSecret *certificates.CertificatesSecret
	if userSecretName := ref.GetClientCertificateSecretName(); userSecretName != "" {
		var err error
		clientCertSecret, err = ReconcileUserProvidedClientCert(ctx, r.Client, association, assocMeta, secretName, userSecretName, extraLabels)
		if err != nil {
			return nil, results.WithError(err)
		}
	} else {
		var certResults *reconciler.Results
		clientCertSecret, certResults = ReconcileManagedClientCert(ctx, r.Client, association, assocMeta, secretName, extraLabels)
		results.WithResults(certResults)
		if certResults.HasError() {
			return nil, results
		}
	}

	if err := certificates.RemoveServerSoftOwnerLabels(ctx, r.Client, &clientCertSecret.Secret); err != nil {
		return nil, results.WithError(err)
	}

	// Publish the public client certificate into the server namespace as the authoritative trust source.
	if err := certificates.ReconcileClientCertTrustSource(
		ctx, r.Client, referencedObj, k8s.ExtractNamespacedName(&clientCertSecret.Secret),
		clientCertSecret.CertPem(), assocMeta,
	); err != nil {
		return nil, results.WithError(err)
	}

	return clientCertSecret, results
}

// ReconcileManagedClientCert creates or updates a self-signed client certificate.
// Returns the client certificate secret and results containing requeue and/or error info.
func ReconcileManagedClientCert(
	ctx context.Context,
	c k8s.Client,
	association commonv1.Association,
	assocMeta metadata.Metadata,
	secretName string,
	extraLabels map[string]string,
) (*certificates.CertificatesSecret, *reconciler.Results) {
	results := reconciler.NewResult(ctx)

	certReconciler := certificates.Reconciler{
		K8sClient: c,
		Owner:     association.Associated(),
		Metadata:  assocMeta,
		CertRotation: certificates.RotationParams{
			Validity:     certificates.DefaultCertValidity,
			RotateBefore: certificates.DefaultRotateBefore,
		},
	}

	commonName := association.Associated().GetName()
	orgUnit := association.Associated().GetName()

	clientCertSecret, err := certReconciler.ReconcileClientCertificate(ctx, secretName, commonName, orgUnit, extraLabels)
	if err != nil {
		return nil, results.WithError(err)
	}

	primaryCert, err := certificates.GetPrimaryCertificate(clientCertSecret.CertPem())
	if err != nil {
		return nil, results.WithError(err)
	}

	results.WithReconciliationState(
		reconciler.
			RequeueAfter(certificates.ShouldRotateIn(time.Now(), primaryCert.NotAfter, certReconciler.CertRotation.RotateBefore)).
			ReconciliationComplete(),
	)
	return clientCertSecret, results
}

// ReconcileUserProvidedClientCert copies a user-provided client certificate secret into the ECK-managed
// secret with the collision-free name, so trust source publication and cleanup work the same for generated and
// user-provided certificates. It returns the reconciled Secret.
func ReconcileUserProvidedClientCert(
	ctx context.Context,
	c k8s.Client,
	association commonv1.Association,
	assocMeta metadata.Metadata,
	targetSecretName string,
	userSecretName string,
	extraLabels map[string]string,
) (*certificates.CertificatesSecret, error) {
	associatedNS := association.GetNamespace()

	// Fetch the user-provided secret (must be in the same namespace as the associated resource).
	userSecret, err := k8s.GetSecretIfExists(ctx, c, types.NamespacedName{
		Namespace: associatedNS,
		Name:      userSecretName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get user-provided client certificate secret %s/%s: %w", associatedNS, userSecretName, err)
	}
	if userSecret == nil {
		return nil, fmt.Errorf("user-provided client certificate secret %s/%s not found", associatedNS, userSecretName)
	}

	// Validate the user-provided secret contains the required keys.
	if _, ok := userSecret.Data[certificates.CertFileName]; !ok {
		return nil, fmt.Errorf("user-provided client certificate secret %s/%s is missing required key %s", associatedNS, userSecretName, certificates.CertFileName)
	}
	if _, ok := userSecret.Data[certificates.KeyFileName]; !ok {
		return nil, fmt.Errorf("user-provided client certificate secret %s/%s is missing required key %s", associatedNS, userSecretName, certificates.KeyFileName)
	}

	// Build expected labels.
	expectedLabels := make(map[string]string)
	maps.Copy(expectedLabels, assocMeta.Labels)
	maps.Copy(expectedLabels, extraLabels)

	// Build expected data.
	expectedData := map[string][]byte{
		certificates.CertFileName: userSecret.Data[certificates.CertFileName],
		certificates.KeyFileName:  userSecret.Data[certificates.KeyFileName],
	}

	expected := corev1.Secret{
		Namespace:   associatedNS,
		Name:        targetSecretName,
		Data:        expectedData,
		Labels:      expectedLabels,
		Annotations: assocMeta.Annotations,
	}

	// Validate the certificate before copying it, so that invalid material ends up in neither the managed Secret nor
	// the client trust bundle.
	clientCertSecret := &certificates.CertificatesSecret{Secret: expected}
	leaf, err := certificates.GetPrimaryCertificate(clientCertSecret.CertPem())
	if err != nil {
		return nil, fmt.Errorf("invalid user-provided client certificate secret %s/%s: %w", associatedNS, userSecretName, err)
	}
	// Check that the key belongs to the leaf. In PKI mode, roles are granted to the leaf's public key, so without
	// this check an association could use another one's certificate.
	// Pass only the leaf: ParsePEMCerts skips PEM blocks with headers and tls.X509KeyPair does not, so with the
	// whole chain they can disagree on which certificate is the leaf.
	if _, err := tls.X509KeyPair(certificates.EncodePEMCert(leaf.Raw), clientCertSecret.KeyPem()); err != nil {
		return nil, fmt.Errorf("invalid user-provided client certificate secret %s/%s: %w", associatedNS, userSecretName, err)
	}

	reconciled, err := reconciler.ReconcileSecret(ctx, c, expected, association.Associated())
	if err != nil {
		return nil, err
	}
	return &certificates.CertificatesSecret{Secret: reconciled}, nil
}

// DeleteClientCertSecret deletes the client certificate secret for the given association if it exists, and its
// trust source in the server namespace.
func DeleteClientCertSecret(ctx context.Context, c k8s.Client, association commonv1.Association, associationName string) error {
	secretName := clientCertSecretName(association, associationName)
	sourceNSN := types.NamespacedName{Namespace: association.GetNamespace(), Name: secretName}
	if err := k8s.DeleteSecretIfExists(ctx, c, sourceNSN); err != nil {
		return err
	}
	return certificates.DeleteClientCertTrustSource(ctx, c, association.AssociationRef().GetNamespace(), sourceNSN)
}
