// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"fmt"
	"maps"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
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

// clientCertSecretName returns the name of the client certificate secret for an association.
// The name is collision-free: it includes a hash of the referenced (server) resource's namespace and name.
func clientCertSecretName(association commonv1.Association, associationName string) string {
	associatedName := association.Associated().GetName()
	ref := association.AssociationRef()
	refHash := hash.HashObject(ref.NamespacedName())
	return fmt.Sprintf("%s-%s-%s-client-cert", associatedName, associationName, refHash)
}

// reconcileClientCertificate reconciles a client certificate for an association when the referenced
// resource requires client authentication. If the user has specified a client certificate secret via
// ClientCertificateSecretName on the association ref, its contents are copied into the ECK-managed secret. Otherwise,
// a self-signed client certificate is auto-generated.
//
// The ECK-managed secret is always created in the associated resource's namespace with soft-owner labels
// pointing to the referenced server resource, enabling trust bundle discovery regardless of the cert source.
//
// Returns the name of the client certificate secret and results containing requeue and/or error info.
func (r *Reconciler) reconcileClientCertificate(
	ctx context.Context,
	association commonv1.Association,
	assocMeta metadata.Metadata,
) (string, *reconciler.Results) {
	results := reconciler.NewResult(ctx)

	secretName := clientCertSecretName(association, r.AssociationName)

	// Build soft-owner labels pointing to the referenced server resource.
	ref := association.AssociationRef()
	extraLabels := map[string]string{
		reconciler.SoftOwnerNameLabel:      ref.GetName(),
		reconciler.SoftOwnerNamespaceLabel: ref.GetNamespace(),
		reconciler.SoftOwnerKindLabel:      r.referencedResourceKind,
		labels.ClientCertificateLabelName:  "true",
	}

	if userSecretName := association.AssociationRef().GetClientCertificateSecretName(); userSecretName != "" {
		if err := ReconcileUserProvidedClientCert(ctx, r.Client, association, assocMeta, secretName, userSecretName, extraLabels); err != nil {
			return "", results.WithError(err)
		}
		return secretName, results
	}

	return ReconcileManagedClientCert(ctx, r.Client, association, assocMeta, secretName, extraLabels)
}

// ReconcileManagedClientCert creates or updates a self-signed client certificate.
func ReconcileManagedClientCert(
	ctx context.Context,
	c k8s.Client,
	association commonv1.Association,
	assocMeta metadata.Metadata,
	secretName string,
	extraLabels map[string]string,
) (string, *reconciler.Results) {
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
		return "", results.WithError(err)
	}

	primaryCert, err := certificates.GetPrimaryCertificate(clientCertSecret.CertPem())
	if err != nil {
		return "", results.WithError(err)
	}

	results.WithReconciliationState(
		reconciler.
			RequeueAfter(certificates.ShouldRotateIn(time.Now(), primaryCert.NotAfter, certReconciler.CertRotation.RotateBefore)).
			ReconciliationComplete(),
	)
	return clientCertSecret.Name, results
}

// ReconcileUserProvidedClientCert copies a user-provided client certificate secret into the ECK-managed
// secret with the collision-free name and soft-owner labels. This ensures the trust bundle discovery
// and cleanup mechanisms work uniformly regardless of whether the cert was auto-generated or user-provided.
func ReconcileUserProvidedClientCert(
	ctx context.Context,
	c k8s.Client,
	association commonv1.Association,
	assocMeta metadata.Metadata,
	targetSecretName string,
	userSecretName string,
	extraLabels map[string]string,
) error {
	associatedNS := association.GetNamespace()

	// Fetch the user-provided secret (must be in the same namespace as the associated resource).
	userSecret, err := k8s.GetSecretIfExists(ctx, c, types.NamespacedName{
		Namespace: associatedNS,
		Name:      userSecretName,
	})
	if err != nil {
		return fmt.Errorf("failed to get user-provided client certificate secret %s/%s: %w", associatedNS, userSecretName, err)
	}
	if userSecret == nil {
		return fmt.Errorf("user-provided client certificate secret %s/%s not found", associatedNS, userSecretName)
	}

	// Validate the user-provided secret contains the required keys.
	if _, ok := userSecret.Data[certificates.CertFileName]; !ok {
		return fmt.Errorf("user-provided client certificate secret %s/%s is missing required key %s", associatedNS, userSecretName, certificates.CertFileName)
	}
	if _, ok := userSecret.Data[certificates.KeyFileName]; !ok {
		return fmt.Errorf("user-provided client certificate secret %s/%s is missing required key %s", associatedNS, userSecretName, certificates.KeyFileName)
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
		ObjectMeta: k8s.ToObjectMeta(types.NamespacedName{Namespace: associatedNS, Name: targetSecretName}),
		Data:       expectedData,
	}
	expected.Labels = expectedLabels
	expected.Annotations = assocMeta.Annotations

	_, err = reconciler.ReconcileSecret(ctx, c, expected, association.Associated())
	return err
}

// DeleteClientCertSecret deletes the client certificate secret for the given association if it exists.
func DeleteClientCertSecret(ctx context.Context, c k8s.Client, association commonv1.Association, associationName string) error {
	secretName := clientCertSecretName(association, associationName)
	return k8s.DeleteSecretIfExists(ctx, c, types.NamespacedName{Namespace: association.GetNamespace(), Name: secretName})
}
