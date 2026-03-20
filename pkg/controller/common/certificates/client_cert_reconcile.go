// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"bytes"
	"cmp"
	"context"
	"crypto"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	utilmaps "github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

const (
	// internalClientCertSecretSuffix is the suffix for the operator client certificate secret.
	internalClientCertSecretSuffix = "internal-client-cert"

	// internalClientCertCommonName is the CommonName used in the operator's client certificate.
	internalClientCertCommonName = "eck-internal"

	// clientCertTrustBundleSecretSuffix is the suffix appended to the owner name to form the trust bundle secret name.
	clientCertTrustBundleSecretSuffix = "client-trust-bundle"
	// ClientCertificatesTrustBundleFileName is the key used in the trust bundle secret data.
	ClientCertificatesTrustBundleFileName = "client-trust-bundle.crt"
)

// OperatorClientCertSecretName returns the expected secret name for the operator client certificate.
func OperatorClientCertSecretName(namer name.Namer, ownerName string) string {
	return namer.Suffix(ownerName, internalClientCertSecretSuffix)
}

// ClientCertTrustBundleSecretName returns the expected secret name for the client certificate trust bundle.
func ClientCertTrustBundleSecretName(namer name.Namer, ownerName string) string {
	return namer.Suffix(ownerName, clientCertTrustBundleSecretSuffix)
}

// ReconcileOperatorCertificate reconciles the operator's internal client certificate used for mTLS with the owner resource.
func (r Reconciler) ReconcileOperatorCertificate(ctx context.Context) (*CertificatesSecret, error) {
	secretName := OperatorClientCertSecretName(r.Namer, r.Owner.GetName())
	return r.ReconcileClientCertificate(ctx, secretName, internalClientCertCommonName, r.Owner.GetName(), nil)
}

// ReconcileClientCertificate reconciles a self-signed client certificate stored in a Kubernetes secret.
// The caller controls the secret name, CommonName, OrganizationalUnit, and any extra labels to set.
// The secret is created if it doesn't exist, and updated only if labels, annotations, or cert data changed.
// Certificate rotation is handled automatically based on the Reconciler's CertRotation params.
func (r Reconciler) ReconcileClientCertificate(
	ctx context.Context,
	secretName string,
	commonName string,
	orgUnit string,
	extraLabels map[string]string,
) (*CertificatesSecret, error) {
	ownerNSN := k8s.ExtractNamespacedName(r.Owner)

	secretNSN := types.NamespacedName{
		Namespace: ownerNSN.Namespace,
		Name:      secretName,
	}

	// Build the expected secret with correct metadata
	expected := corev1.Secret{
		ObjectMeta: k8s.ToObjectMeta(secretNSN),
		Data:       make(map[string][]byte),
	}
	expected.Labels = utilmaps.Merge(r.Metadata.Labels, extraLabels)
	expected.Annotations = r.Metadata.Annotations

	// Seed with existing data so ensureClientCertificateSecretContents can
	// reuse still-valid certificates and keys instead of regenerating them.
	if existing, err := k8s.GetSecretIfExists(ctx, r.K8sClient, secretNSN); err != nil {
		return nil, err
	} else if existing != nil {
		expected.Data = existing.Data
	}

	// Check and update certificate data
	if err := ensureClientCertificateSecretContents(ctx, &expected, commonName, orgUnit, r.CertRotation); err != nil {
		return nil, err
	}

	reconciledSecret, err := reconciler.ReconcileSecret(ctx, r.K8sClient, expected, r.Owner)
	if err != nil {
		return nil, err
	}
	return &CertificatesSecret{
		Secret: reconciledSecret,
	}, nil
}

// ReconcileTrustBundle builds a trust bundle containing:
// 1. The operator client certificate (passed directly, owned by the server resource)
// 2. Any association client certificates (discovered via soft-owner labels)
//
// The trust bundle secret contains concatenated PEM-encoded certificates for client certificate validation.
func (r Reconciler) ReconcileTrustBundle(ctx context.Context, ownerKind string, extraCertificates ...*CertificatesSecret) error {
	ownerNSN := k8s.ExtractNamespacedName(r.Owner)
	secretName := ClientCertTrustBundleSecretName(r.Namer, ownerNSN.Name)

	// Start with the operator client certificate (directly owned, no soft-owner labels needed)
	var allSecrets []corev1.Secret
	for _, extraCert := range extraCertificates {
		allSecrets = append(allSecrets, extraCert.Secret)
	}

	// Discover association client certificate secrets with matching soft-owner labels
	associationSecrets, err := discoverClientCertSecrets(ctx, r.K8sClient, ownerNSN.Name, ownerNSN.Namespace, ownerKind)
	if err != nil {
		return fmt.Errorf("failed to discover client certificate secrets: %w", err)
	}
	allSecrets = append(allSecrets, associationSecrets...)

	// Build trust bundle from all secrets
	bundleData := buildTrustBundleFromSecrets(ctx, allSecrets)

	expected := corev1.Secret{
		ObjectMeta: k8s.ToObjectMeta(types.NamespacedName{
			Namespace: ownerNSN.Namespace,
			Name:      secretName,
		}),
		Data: map[string][]byte{
			ClientCertificatesTrustBundleFileName: bundleData,
		},
	}
	expected.Labels = r.Metadata.Labels
	expected.Annotations = r.Metadata.Annotations

	_, err = reconciler.ReconcileSecret(ctx, r.K8sClient, expected, r.Owner)
	return err
}

// discoverClientCertSecrets lists all secrets across namespaces that are labeled as client certificates
// with soft-owner labels matching the specified owner.
func discoverClientCertSecrets(ctx context.Context, c k8s.Client, ownerName, ownerNamespace, ownerKind string) ([]corev1.Secret, error) {
	log := ulog.FromContext(ctx)

	var clientCertificateSecrets corev1.SecretList
	if err := c.List(ctx, &clientCertificateSecrets, client.MatchingLabels{
		reconciler.SoftOwnerNameLabel:      ownerName,
		reconciler.SoftOwnerNamespaceLabel: ownerNamespace,
		reconciler.SoftOwnerKindLabel:      ownerKind,
		labels.ClientCertificateLabelName:  "true",
	}); err != nil {
		return nil, fmt.Errorf("failed to list client certificate secrets: %w", err)
	}

	log.V(1).Info("Discovered client certificate secrets",
		"owner_name", ownerName,
		"owner_namespace", ownerNamespace,
		"owner_kind", ownerKind,
		"count", len(clientCertificateSecrets.Items))

	return clientCertificateSecrets.Items, nil
}

// buildTrustBundleFromSecrets extracts client certificates from the given secrets and concatenates them.
// The output is sorted by secret namespace/name for deterministic results.
func buildTrustBundleFromSecrets(ctx context.Context, secrets []corev1.Secret) []byte {
	log := ulog.FromContext(ctx)

	slices.SortFunc(secrets, func(a, b corev1.Secret) int {
		return cmp.Or(
			strings.Compare(a.Namespace, b.Namespace),
			strings.Compare(a.Name, b.Name),
		)
	})

	var buf bytes.Buffer
	for _, secret := range secrets {
		certData := secret.Data[CertFileName]
		if len(certData) == 0 {
			log.V(1).Info("Skipping secret with no certificate data",
				"namespace", secret.Namespace, "name", secret.Name)
			continue
		}
		buf.Write(certData)
		if len(certData) > 0 && certData[len(certData)-1] != '\n' {
			buf.WriteByte('\n')
		}
	}

	return buf.Bytes()
}

// ensureClientCertificateSecretContents ensures the secret contains valid client certificate data.
// It modifies the secret in place.
func ensureClientCertificateSecretContents(
	ctx context.Context,
	secret *corev1.Secret,
	commonName string,
	orgUnit string,
	rotationParam RotationParams,
) error {
	log := ulog.FromContext(ctx)
	privateKey := privateKeyIfValid(ctx, secret)
	if privateKey == nil {
		generatedPrivateKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
		if err != nil {
			return err
		}
		privateKey = generatedPrivateKey

		// Use PKCS#8 encoding for client certificate keys to ensure compatibility with
		// all elastic stack components
		// Note: Some Java-based applications, such as Logstash, require PKCS#8 format.
		encodedKey, err := EncodePEMPKCS8PrivateKey(privateKey)
		if err != nil {
			return err
		}
		secret.Data[KeyFileName] = encodedKey
	}

	existingCertBytes := clientCertificateIfValid(ctx, secret, commonName, rotationParam.RotateBefore)
	if existingCertBytes == nil {
		log.Info(
			"Issuing new client certificate",
			"namespace", secret.Namespace,
			"secret_name", secret.Name,
			"common_name", commonName,
		)

		certTemplate := createClientCertTemplate(commonName, orgUnit, privateKey.Public(), rotationParam.Validity)
		certBytes, err := createSelfSignedClientCert(*certTemplate, privateKey)
		if err != nil {
			return err
		}

		secret.Data[CertFileName] = EncodePEMCert(certBytes)
	}

	delete(secret.Data, CAFileName)

	return nil
}

// clientCertificateIfValid returns the client certificate from the provided Secret if it's valid.
// Returns nil if the certificate needs to be re-issued.
func clientCertificateIfValid(
	ctx context.Context,
	secret *corev1.Secret,
	commonName string,
	certReconcileBefore time.Duration,
) []byte {
	log := ulog.FromContext(ctx)

	certData, ok := secret.Data[CertFileName]
	if !ok || len(certData) == 0 {
		return nil
	}

	certs, err := ParsePEMCerts(certData)
	if err != nil {
		log.Error(err, "Invalid certificate data found, issuing new certificate",
			"namespace", secret.Namespace, "secret_name", secret.Name)
		return nil
	}

	if len(certs) == 0 {
		return nil
	}

	cert := certs[0]

	if cert.Subject.CommonName != commonName {
		log.V(1).Info("Certificate CommonName mismatch, will re-issue",
			"expected", commonName, "actual", cert.Subject.CommonName)
		return nil
	}

	if !CertIsValid(ctx, *cert, certReconcileBefore) {
		return nil
	}

	privateKey := privateKeyIfValid(ctx, secret)
	if privateKey == nil {
		log.V(1).Info("No valid private key in secret, will re-issue",
			"namespace", secret.Namespace, "secret_name", secret.Name)
		return nil
	}
	if !PrivateMatchesPublicKey(ctx, cert.PublicKey, privateKey) {
		log.V(1).Info("Certificate public key does not match private key in secret, will re-issue",
			"namespace", secret.Namespace, "secret_name", secret.Name)
		return nil
	}

	return cert.Raw
}

// privateKeyIfValid parses the existing private key from the secret.
// Returns nil if the key is missing or cannot be parsed.
func privateKeyIfValid(ctx context.Context, secret *corev1.Secret) crypto.Signer {
	keyPEM, ok := secret.Data[KeyFileName]
	if !ok || len(keyPEM) == 0 {
		return nil
	}
	key, err := ParsePEMPrivateKey(keyPEM)
	if err != nil {
		ulog.FromContext(ctx).Error(err, "Failed to parse existing private key, will generate new one")
		return nil
	}
	return key
}

func createSelfSignedClientCert(template ValidatedCertificateTemplate, privateKey crypto.Signer) ([]byte, error) {
	serial, err := cryptorand.Int(cryptorand.Reader, SerialNumberLimit)
	if err != nil {
		return nil, err
	}
	certTemplate := x509.Certificate(template)
	certTemplate.SerialNumber = serial

	certData, err := x509.CreateCertificate(
		cryptorand.Reader,
		&certTemplate,
		&certTemplate,
		privateKey.Public(),
		privateKey,
	)
	return certData, err
}

func createClientCertTemplate(
	commonName string,
	orgUnit string,
	publicKey crypto.PublicKey,
	validity time.Duration,
) *ValidatedCertificateTemplate {
	template := ValidatedCertificateTemplate(x509.Certificate{
		Subject: pkix.Name{
			CommonName:         commonName,
			OrganizationalUnit: []string{orgUnit},
		},
		NotBefore: time.Now().Add(-10 * time.Minute),
		NotAfter:  time.Now().Add(validity),
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
		},
		PublicKey: publicKey,
	})
	return &template
}

// ParseTLSCertificate parses a tls.Certificate from a secret containing tls.crt and tls.key.
func ParseTLSCertificate(secret corev1.Secret) (*tls.Certificate, error) {
	certPEM, ok := secret.Data[CertFileName]
	if !ok || len(certPEM) == 0 {
		return nil, fmt.Errorf("missing %s in secret %s/%s", CertFileName, secret.Namespace, secret.Name)
	}
	keyPEM, ok := secret.Data[KeyFileName]
	if !ok || len(keyPEM) == 0 {
		return nil, fmt.Errorf("missing %s in secret %s/%s", KeyFileName, secret.Namespace, secret.Name)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parsing %s and %s: %w", CertFileName, KeyFileName, err)
	}
	return &cert, nil
}

// LoadOperatorClientCertIfExists loads a tls.Certificate from a secret if it exists.
// Returns nil (without error) if the secret does not exist.
func LoadOperatorClientCertIfExists(ctx context.Context, c k8s.Client, namer name.Namer, namespace, ownerName string) (*tls.Certificate, error) {
	secretName := OperatorClientCertSecretName(namer, ownerName)
	secret, err := k8s.GetSecretIfExists(ctx, c, types.NamespacedName{Namespace: namespace, Name: secretName})
	if err != nil {
		return nil, err
	}
	if secret == nil {
		return nil, nil
	}
	return ParseTLSCertificate(*secret)
}
