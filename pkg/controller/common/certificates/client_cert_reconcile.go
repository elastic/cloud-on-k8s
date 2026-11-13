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
	"maps"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
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

	// ClientCertTrustSourceType is the type label value of the trust source secrets published into the server
	// namespace. The server's client certificate trust bundle is built from these secrets only.
	ClientCertTrustSourceType = "client-cert-trust-source"

	// clientCertTrustSourceSuffix is the suffix of the trust source secret names.
	clientCertTrustSourceSuffix = "client-cert-source"

	// clientCertSourceNamespaceAnnotation and clientCertSourceNameAnnotation record the originating client
	// certificate secret on a trust source. The trust bundle is ordered by it, which keeps the bundle bytes
	// unchanged for existing clusters.
	clientCertSourceNamespaceAnnotation = "certificates.k8s.elastic.co/client-cert-source-namespace"
	clientCertSourceNameAnnotation      = "certificates.k8s.elastic.co/client-cert-source-name"
)

// clientCertTrustSourceNamer names trust source secrets using DNS subdomain naming (253 chars max).
var clientCertTrustSourceNamer = name.NewSecretNamer()

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
		Namespace:   ownerNSN.Namespace,
		Name:        secretName,
		Data:        make(map[string][]byte),
		Labels:      utilmaps.Merge(maps.Clone(r.Metadata.Labels), extraLabels),
		Annotations: maps.Clone(r.Metadata.Annotations),
	}

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
// 2. Any association client certificates published as trust source records into the server namespace.
//
// The trust bundle secret contains concatenated PEM-encoded certificates for client certificate validation.
func (r Reconciler) ReconcileTrustBundle(ctx context.Context, extraCertificates ...*CertificatesSecret) ([]byte, error) {
	ownerNSN := k8s.ExtractNamespacedName(r.Owner)
	secretName := ClientCertTrustBundleSecretName(r.Namer, ownerNSN.Name)

	// Start with the certificates passed in by the caller (the operator client certificate).
	var allSecrets []corev1.Secret
	for _, extraCert := range extraCertificates {
		allSecrets = append(allSecrets, extraCert.Secret)
	}

	// Only the trust sources owned by the server in its own namespace are used. The operator publishes them once
	// the association of the client with the server is reconciled.
	sources, err := listClientCertTrustSources(ctx, r.K8sClient, r.Owner)
	if err != nil {
		return nil, fmt.Errorf("failed to list client certificate trust sources: %w", err)
	}
	allSecrets = append(allSecrets, sources...)

	// Build trust bundle from all secrets
	bundleData := buildTrustBundleFromSecrets(ctx, allSecrets)

	expected := corev1.Secret{
		Namespace: ownerNSN.Namespace,
		Name:      secretName,
		Data: map[string][]byte{
			ClientCertificatesTrustBundleFileName: bundleData,
		},
		Labels:      maps.Clone(r.Metadata.Labels),
		Annotations: maps.Clone(r.Metadata.Annotations),
	}

	_, err = reconciler.ReconcileSecret(ctx, r.K8sClient, expected, r.Owner)
	return bundleData, err
}

// clientCertTrustSourceSecretName returns the deterministic name of the trust source secret published into
// the server namespace for a client certificate originating from the given source secret. The hash of the
// source namespace and name keeps it collision-free across associated resources that share a name.
func clientCertTrustSourceSecretName(sourceNamespace, sourceName string) string {
	refHash := hash.HashObject(types.NamespacedName{Namespace: sourceNamespace, Name: sourceName})
	return clientCertTrustSourceNamer.Suffix(sourceName, refHash, clientCertTrustSourceSuffix)
}

// ReconcileClientCertTrustSource publishes the public part of a client certificate into the server namespace so
// the server includes it in its client certificate trust bundle. It must only be called once the client's access
// to the server is authorized (RBAC-on-refs). The server owns the record, which inherits the labels in meta so the
// publishing resource can find and delete it.
func ReconcileClientCertTrustSource(
	ctx context.Context,
	c k8s.Client,
	server client.Object,
	sourceNSN types.NamespacedName,
	certPEM []byte,
	meta metadata.Metadata,
) error {
	secretLabels := maps.Clone(meta.Labels)
	if secretLabels == nil {
		secretLabels = map[string]string{}
	}
	// The client certificate label is reserved for the client certificate secret in the component namespace.
	delete(secretLabels, labels.ClientCertificateLabelName)
	secretLabels[commonv1.TypeLabelName] = ClientCertTrustSourceType

	secretAnnotations := maps.Clone(meta.Annotations)
	if secretAnnotations == nil {
		secretAnnotations = map[string]string{}
	}
	// Record the originating secret so the trust bundle can preserve a stable, source-based ordering.
	secretAnnotations[clientCertSourceNamespaceAnnotation] = sourceNSN.Namespace
	secretAnnotations[clientCertSourceNameAnnotation] = sourceNSN.Name

	expected := corev1.Secret{
		Namespace:   server.GetNamespace(),
		Name:        clientCertTrustSourceSecretName(sourceNSN.Namespace, sourceNSN.Name),
		Data:        map[string][]byte{CertFileName: certPEM},
		Labels:      secretLabels,
		Annotations: secretAnnotations,
	}
	// Also update a trust source controlled by a previous server of the same name, which has not been garbage
	// collected yet: ReconcileSecret does not compare owner references. Left alone, it would be ignored as soon as
	// the garbage collector removes it.
	needsReOwning := func(p *reconciler.Params) {
		needsUpdate := p.NeedsUpdate
		p.NeedsUpdate = func() bool {
			return needsUpdate() || !metav1.IsControlledBy(p.Reconciled, server)
		}
	}
	_, err := reconciler.ReconcileSecret(ctx, c, expected, server, needsReOwning)
	return err
}

// ClientCertTrustSourceKey returns the key of the trust source published into the server namespace for the client
// certificate secret sourceNSN.
func ClientCertTrustSourceKey(serverNamespace string, sourceNSN types.NamespacedName) types.NamespacedName {
	return types.NamespacedName{
		Namespace: serverNamespace,
		Name:      clientCertTrustSourceSecretName(sourceNSN.Namespace, sourceNSN.Name),
	}
}

// DeleteClientCertTrustSource removes the trust source record published for the given client certificate
// source from the server namespace, if it exists.
func DeleteClientCertTrustSource(ctx context.Context, c k8s.Client, serverNamespace string, sourceNSN types.NamespacedName) error {
	return k8s.DeleteSecretIfExists(ctx, c, ClientCertTrustSourceKey(serverNamespace, sourceNSN))
}

// ClientCertTrustSourceOrigin returns the namespace and name of the client certificate secret the given trust
// source record was published from. It returns false if the secret is not a trust source record.
func ClientCertTrustSourceOrigin(secret corev1.Secret) (types.NamespacedName, bool) {
	namespace, hasNamespace := secret.Annotations[clientCertSourceNamespaceAnnotation]
	name, hasName := secret.Annotations[clientCertSourceNameAnnotation]
	if !hasNamespace || !hasName {
		return types.NamespacedName{}, false
	}
	return types.NamespacedName{Namespace: namespace, Name: name}, true
}

// ListClientCertTrustSources lists, across all namespaces, the trust sources carrying the given labels. They
// inherit the labels of the resource that published them, wherever they were published.
func ListClientCertTrustSources(ctx context.Context, c k8s.Client, matchingLabels client.MatchingLabels) ([]corev1.Secret, error) {
	selector := client.MatchingLabels{commonv1.TypeLabelName: ClientCertTrustSourceType}
	maps.Copy(selector, matchingLabels)

	var secrets corev1.SecretList
	if err := c.List(ctx, &secrets, selector); err != nil {
		return nil, fmt.Errorf("failed to list client certificate trust sources: %w", err)
	}
	return secrets.Items, nil
}

// RemoveServerSoftOwnerLabels removes the server soft-owner labels that older operator versions set on client
// certificate secrets. They are no longer used. Only labels change, so the client's pods do not roll.
func RemoveServerSoftOwnerLabels(ctx context.Context, c k8s.Client, secret *corev1.Secret) error {
	var patchBase client.Patch
	for _, l := range []string{reconciler.SoftOwnerNameLabel, reconciler.SoftOwnerNamespaceLabel, reconciler.SoftOwnerKindLabel} {
		if _, ok := secret.Labels[l]; !ok {
			continue
		}
		if patchBase == nil {
			// a label has to be removed: snapshot the secret before deleting
			// the first one, so the patch can compute the diff
			patchBase = client.MergeFrom(secret.DeepCopy())
		}
		delete(secret.Labels, l)
	}
	if patchBase == nil {
		return nil
	}
	return c.Patch(ctx, secret, patchBase)
}

// listClientCertTrustSources lists the trust sources published into the namespace of the given server, and
// controlled by it.
func listClientCertTrustSources(ctx context.Context, c k8s.Client, server client.Object) ([]corev1.Secret, error) {
	var secrets corev1.SecretList
	if err := c.List(ctx, &secrets,
		client.InNamespace(server.GetNamespace()),
		client.MatchingLabels{commonv1.TypeLabelName: ClientCertTrustSourceType},
	); err != nil {
		return nil, fmt.Errorf("failed to list client certificate trust sources: %w", err)
	}

	trustSources := make([]corev1.Secret, 0, len(secrets.Items))
	for _, secret := range secrets.Items {
		if metav1.IsControlledBy(&secret, server) {
			trustSources = append(trustSources, secret)
		}
	}

	ulog.FromContext(ctx).V(1).Info("Listed client certificate trust sources",
		"server_name", server.GetName(),
		"server_namespace", server.GetNamespace(),
		"count", len(trustSources))

	return trustSources, nil
}

// buildTrustBundleFromSecrets extracts client certificates from the given secrets and concatenates them.
// The output is sorted by the originating client certificate's namespace/name (recorded on published trust
// sources) for deterministic results that stay stable regardless of where the trust source is stored.
func buildTrustBundleFromSecrets(ctx context.Context, secrets []corev1.Secret) []byte {
	log := ulog.FromContext(ctx)

	// sortKey returns the originating secret identity when recorded (trust source records), falling back to the
	// secret's own namespace/name (e.g. the operator client certificate passed directly).
	sortKey := func(s corev1.Secret) (string, string) {
		if ns, ok := s.Annotations[clientCertSourceNamespaceAnnotation]; ok {
			return ns, s.Annotations[clientCertSourceNameAnnotation]
		}
		return s.Namespace, s.Name
	}
	slices.SortFunc(secrets, func(a, b corev1.Secret) int {
		aNS, aName := sortKey(a)
		bNS, bName := sortKey(b)
		return cmp.Or(
			strings.Compare(aNS, bNS),
			strings.Compare(aName, bName),
		)
	})

	var buf bytes.Buffer
	for _, secret := range secrets {
		certData := secret.Data[CertFileName]
		if len(certData) == 0 {
			log.V(1).Info("Skipping secret with no certificate data",
				"secret_namespace", secret.Namespace, "secret_name", secret.Name)
			continue
		}
		buf.Write(certData)
		if certData[len(certData)-1] != '\n' {
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

	if isClientCertificateValid(ctx, secret, commonName, rotationParam.RotateBefore) {
		return nil
	}

	log.Info(
		"Issuing new client certificate",
		"secret_namespace", secret.Namespace,
		"secret_name", secret.Name,
		"common_name", commonName,
	)

	privateKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	if err != nil {
		return err
	}

	// Use PKCS#8 encoding for client certificate keys to ensure compatibility with
	// all Elastic Stack components.
	// Note: Some Java-based applications, such as Logstash, require PKCS#8 format.
	encodedKey, err := EncodePEMPKCS8PrivateKey(privateKey)
	if err != nil {
		return err
	}
	secret.Data[KeyFileName] = encodedKey

	certTemplate := createClientCertTemplate(commonName, orgUnit, privateKey.Public(), rotationParam.Validity)
	certBytes, err := createSelfSignedClientCert(*certTemplate, privateKey)
	if err != nil {
		return err
	}

	secret.Data[CertFileName] = EncodePEMCert(certBytes)

	return nil
}

// isClientCertificateValid returns true if the secret contains a valid client certificate
// and private key pair that does not need to be re-issued.
func isClientCertificateValid(
	ctx context.Context,
	secret *corev1.Secret,
	commonName string,
	certReconcileBefore time.Duration,
) bool {
	log := ulog.FromContext(ctx)

	privateKey := privateKeyIfValid(ctx, secret)
	if privateKey == nil {
		return false
	}

	certData, ok := secret.Data[CertFileName]
	if !ok || len(certData) == 0 {
		return false
	}

	certs, err := ParsePEMCerts(certData)
	if err != nil {
		log.Error(err, "Invalid certificate data found, issuing new certificate",
			"secret_namespace", secret.Namespace, "secret_name", secret.Name)
		return false
	}

	if len(certs) == 0 {
		return false
	}

	cert := certs[0]

	if cert.Subject.CommonName != commonName {
		log.V(1).Info("Certificate CommonName mismatch, will re-issue",
			"expected", commonName, "actual", cert.Subject.CommonName)
		return false
	}

	if !CertIsValid(ctx, *cert, certReconcileBefore) {
		return false
	}

	if !PrivateMatchesPublicKey(ctx, cert.PublicKey, privateKey) {
		log.V(1).Info("Certificate public key does not match private key in secret, will re-issue",
			"secret_namespace", secret.Namespace, "secret_name", secret.Name)
		return false
	}

	return true
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
		ulog.FromContext(ctx).Error(err, "Failed to parse private key",
			"secret_namespace", secret.Namespace, "secret_name", secret.Name)
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
		BasicConstraintsValid: true,
		IsCA:                  false,
		PublicKey:             publicKey,
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
	if err != nil || secret == nil {
		return nil, err
	}
	return ParseTLSCertificate(*secret)
}

// DeleteClientCertResources deletes the client certificate resources (operator client cert secret,
// trust bundle secret, the trust sources the owner controls, and the client-authentication-required
// annotation) when client certificate authentication is no longer required. This should only be called
// after all pods have rolled to the new configuration without client authentication.
func DeleteClientCertResources(ctx context.Context, c k8s.Client, owner client.Object, namer name.Namer) error {
	ownerNSN := k8s.ExtractNamespacedName(owner)

	// Remove the annotation first: orphaned secrets are harmless (GC'd via owner references),
	// but an orphaned annotation actively misleads association controllers into thinking
	// client certificates are still required.
	if err := annotation.RemoveClientAuthenticationRequiredAnnotation(ctx, c, owner); err != nil {
		return err
	}

	// Delete the operator client certificate secret
	if err := k8s.DeleteSecretIfExists(ctx, c, types.NamespacedName{
		Namespace: ownerNSN.Namespace,
		Name:      OperatorClientCertSecretName(namer, ownerNSN.Name),
	}); err != nil {
		return err
	}

	// Delete the client certificate trust bundle secret
	if err := k8s.DeleteSecretIfExists(ctx, c, types.NamespacedName{
		Namespace: ownerNSN.Namespace,
		Name:      ClientCertTrustBundleSecretName(namer, ownerNSN.Name),
	}); err != nil {
		return err
	}

	// Delete the trust sources published for the owner, without relying on every client to reconcile and delete its
	// own. The clients publish them again if client authentication is required again.
	trustSources, err := listClientCertTrustSources(ctx, c, owner)
	if err != nil {
		return err
	}
	for i := range trustSources {
		if err := k8s.DeleteSecretIfExists(ctx, c, k8s.ExtractNamespacedName(&trustSources[i])); err != nil {
			return err
		}
	}

	return nil
}
