// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"net"
	"reflect"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	netutil "github.com/elastic/cloud-on-k8s/v2/pkg/utils/net"
)

// ReconcilePublicHTTPCerts reconciles the Secret containing the HTTP Certificate currently in use, and the CA of
// the certificate if available.
func (r Reconciler) ReconcilePublicHTTPCerts(ctx context.Context, internalCerts *CertificatesSecret) error {
	nsn := PublicCertsSecretRef(r.Namer, k8s.ExtractNamespacedName(r.Owner))
	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: nsn.Namespace,
			Name:      nsn.Name,
			Labels:    r.Labels,
		},
		Data: map[string][]byte{
			CertFileName: internalCerts.CertPem(),
		},
	}
	if caPem := internalCerts.CAPem(); caPem != nil {
		expected.Data[CAFileName] = caPem
	}

	// Don't set an ownerRef for public http certs secrets, likely to be copied into different namespaces.
	// See https://github.com/elastic/cloud-on-k8s/issues/3986.
	_, err := reconciler.ReconcileSecretNoOwnerRef(ctx, r.K8sClient, expected, r.Owner)
	return err
}

// ReconcileInternalHTTPCerts reconciles the internal resources for the HTTP certificate.
func (r Reconciler) ReconcileInternalHTTPCerts(ctx context.Context, ca *CA, customCertificates *CertificatesSecret) (*CertificatesSecret, error) {
	log := ulog.FromContext(ctx)
	ownerNSN := k8s.ExtractNamespacedName(r.Owner)

	watchKey := CertificateWatchKey(r.Namer, ownerNSN.Name)
	if err := ReconcileCustomCertWatch(r.DynamicWatches, watchKey, ownerNSN, r.TLSOptions.Certificate); err != nil {
		return nil, err
	}

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ownerNSN.Namespace,
			Name:      InternalCertsSecretName(r.Namer, ownerNSN.Name),
		},
	}

	shouldCreateSecret := false
	if err := r.K8sClient.Get(ctx, k8s.ExtractNamespacedName(&secret), &secret); err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	} else if apierrors.IsNotFound(err) {
		shouldCreateSecret = true
	}

	if secret.Labels == nil {
		secret.Labels = make(map[string]string)
	}

	// TODO: reconcile annotations?
	needsUpdate := false

	// ensure our labels are set on the secret.
	for k, v := range r.Labels {
		if current, ok := secret.Labels[k]; !ok || current != v {
			secret.Labels[k] = v
			needsUpdate = true
		}
	}

	if err := controllerutil.SetControllerReference(r.Owner, &secret, scheme.Scheme); err != nil {
		return nil, err
	}

	// a placeholder secret may have nil entries, create them if needed
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}

	// by default let's assume that the CA is provided, either by the ECK internal certificate authority or by the user
	caCertProvided := true

	if customCertificates.HasLeafCertificate() {
		caCertProvided, needsUpdate = r.populateFromCustomCertificateContents(&secret, customCertificates, ca)
	} else {
		selfSignedNeedsUpdate, err := ensureInternalSelfSignedCertificateSecretContents(
			ctx, &secret, ownerNSN, r.Namer, r.TLSOptions, r.ExtraHTTPSANs, r.Services, ca, r.CertRotation,
		)
		if err != nil {
			return nil, err
		}
		needsUpdate = needsUpdate || selfSignedNeedsUpdate
	}

	//nolint:nestif
	if needsUpdate {
		if shouldCreateSecret {
			log.Info("Creating HTTP internal certificate secret", "namespace", secret.Namespace, "secret_name", secret.Name)
			if err := r.K8sClient.Create(ctx, &secret); err != nil {
				return nil, err
			}
		} else {
			log.Info("Updating HTTP internal certificate secret", "namespace", secret.Namespace, "secret_name", secret.Name)
			if err := r.K8sClient.Update(ctx, &secret); err != nil {
				return nil, err
			}
		}
	}

	// The CA cert has been set in this Secret for convenience, remove it from the result in order to not propagate it.
	if !caCertProvided {
		delete(secret.Data, CAFileName)
	}

	internalCerts := CertificatesSecret{Secret: secret}
	return &internalCerts, nil
}

// populateFromCustomCertificateContents populates the secret passed as a parameter from the contents of customCertificates. Returns two
// booleans: whether a CA certificate has been provided through the custom certificate secret and secondly whether data in the resulting secret
// has been updated.
func (r Reconciler) populateFromCustomCertificateContents(secret *corev1.Secret, customCertificates *CertificatesSecret, ca *CA) (bool, bool) {
	caCertProvided := true
	expectedSecretData := make(map[string][]byte)
	expectedSecretData[CertFileName] = customCertificates.CertPem()
	expectedSecretData[KeyFileName] = customCertificates.KeyPem()
	switch {
	case customCertificates.HasCA():
		expectedSecretData[CAFileName] = customCertificates.CAPem()
	case r.DisableInternalCADefaulting:
		// NOOP
	default:
		// Ensure that the CA certificate is never empty, otherwise Elasticsearch is not able to reload the certificates.
		// Default to our self-signed (useless) CA if none is provided by the user.
		// See https://github.com/elastic/cloud-on-k8s/issues/2243
		expectedSecretData[CAFileName] = EncodePEMCert(ca.Cert.Raw)
		// The CA has been set in the internal HTTP secret but it's only for convenience, in order to circumvent the
		// aforementioned issue. We need to remove it later from the result.
		caCertProvided = false
	}
	if !reflect.DeepEqual(secret.Data, expectedSecretData) {
		secret.Data = expectedSecretData
		return caCertProvided, true
	}
	return caCertProvided, false
}

// ensureInternalSelfSignedCertificateSecretContents ensures that contents of a secret containing self-signed
// certificates is valid. The provided secret is updated in-place.
//
// Returns true if the secret was changed.
func ensureInternalSelfSignedCertificateSecretContents(
	ctx context.Context,
	secret *corev1.Secret,
	owner types.NamespacedName,
	namer name.Namer,
	tls commonv1.TLSOptions,
	controllerSANs []commonv1.SubjectAlternativeName,
	svcs []corev1.Service,
	ca *CA,
	rotationParam RotationParams,
) (bool, error) {
	log := ulog.FromContext(ctx)
	secretWasChanged := false

	// verify that the secret contains a parsable and compatible private key
	privateKey := GetCompatiblePrivateKey(ctx, ca.PrivateKey, secret, KeyFileName)

	// if we need a new private key, generate it
	if privateKey == nil {
		generatedPrivateKey, err := NewPrivateKey(ca.PrivateKey)
		if err != nil {
			return false, err
		}
		encodedPEM, err := EncodePEMPrivateKey(generatedPrivateKey)
		if err != nil {
			return false, err
		}
		secret.Data[KeyFileName] = encodedPEM
		privateKey = generatedPrivateKey
		secretWasChanged = true
	}

	// check if the existing cert should be re-issued
	certificate := getHTTPCertificate(ctx, owner, namer, tls, controllerSANs, secret, svcs, ca, rotationParam.RotateBefore)
	if certificate == nil {
		log.Info(
			"Issuing new HTTP certificate",
			"namespace", secret.Namespace,
			"secret_name", secret.Name,
			"owner_namespace", owner.Namespace,
			"owner_name", owner.Name,
		)

		csr, err := x509.CreateCertificateRequest(cryptorand.Reader, &x509.CertificateRequest{}, privateKey)
		if err != nil {
			return secretWasChanged, err
		}

		// create a cert from the csr
		parsedCSR, err := x509.ParseCertificateRequest(csr)
		if err != nil {
			return secretWasChanged, err
		}

		// validate the csr
		validatedCertificateTemplate := createValidatedHTTPCertificateTemplate(
			owner, namer, tls, controllerSANs, svcs, parsedCSR, rotationParam.Validity,
		)
		// sign the certificate
		certificate, err = ca.CreateCertificate(*validatedCertificateTemplate)
		if err != nil {
			return secretWasChanged, err
		}

		secretWasChanged = true
		// store certificate and signed certificate in a secret mounted into the pod
		secret.Data[CAFileName] = EncodePEMCert(ca.Cert.Raw)
		secret.Data[CertFileName] = EncodePEMCert(certificate, ca.Cert.Raw)
	}

	// Ensure that the CA certificate is up-to-date.
	expectedCaPem := EncodePEMCert(ca.Cert.Raw)
	expectedCertPem := EncodePEMCert(certificate, ca.Cert.Raw)
	if !reflect.DeepEqual(secret.Data[CAFileName], expectedCaPem) || !reflect.DeepEqual(secret.Data[CertFileName], expectedCertPem) {
		log.Info(
			"Updating CA certificate",
			"secret_name", secret.Name,
			"namespace", secret.Namespace,
			"owner_name", owner.Name,
		)
		secretWasChanged = true
		secret.Data[CAFileName] = expectedCaPem
		secret.Data[CertFileName] = expectedCertPem
	}

	return secretWasChanged, nil
}

// getHTTPCertificate returns the HTTP certificate from the provided Secret. It returns nil if the certificate does not
// exist or is not valid, in which case we should issue a new HTTP certificate.
//
// Reasons for considering a certificate as invalid:
//
//   - no certificate yet
//   - certificate has the wrong format
//   - certificate is invalid according to the CA or expired
//   - certificate SAN and IP does not match the expected ones
func getHTTPCertificate(
	ctx context.Context,
	owner types.NamespacedName,
	namer name.Namer,
	tls commonv1.TLSOptions,
	controllerSANs []commonv1.SubjectAlternativeName,
	secret *corev1.Secret,
	svcs []corev1.Service,
	ca *CA,
	certReconcileBefore time.Duration,
) []byte {
	log := ulog.FromContext(ctx)

	validatedTemplate := createValidatedHTTPCertificateTemplate(
		owner, namer, tls, controllerSANs, svcs, &x509.CertificateRequest{}, certReconcileBefore,
	)

	var certificate *x509.Certificate

	certData, ok := secret.Data[CertFileName]
	if !ok {
		return nil
	}
	certs, err := ParsePEMCerts(certData)
	if err != nil {
		log.Error(err, "Invalid certificate data found, issuing new certificate", "namespace", secret.Namespace, "secret_name", secret.Name)
		return nil
	}

	// look for the certificate based on the CommonName
	for _, c := range certs {
		if c.Subject.CommonName == validatedTemplate.Subject.CommonName {
			certificate = c
			break
		}
	}

	if certificate == nil {
		return nil
	}

	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	verifyOpts := x509.VerifyOptions{
		DNSName:       validatedTemplate.Subject.CommonName,
		Roots:         pool,
		Intermediates: pool,
	}
	if _, err := certificate.Verify(verifyOpts); err != nil {
		log.Info(
			"Certificate was not valid, should issue new",
			"validation_failure", err,
			"subject", certificate.Subject,
			"issuer", certificate.Issuer,
			"current_ca_subject", ca.Cert.Subject,
			"secret_name", secret.Name,
			"namespace", secret.Namespace,
			"owner_name", owner.Name,
		)
		return nil
	}

	if time.Now().After(certificate.NotAfter.Add(-certReconcileBefore)) {
		log.Info("Certificate soon to expire, should issue new", "namespace", secret.Namespace, "secret_name", secret.Name)
		return nil
	}

	if certificate.Subject.String() != validatedTemplate.Subject.String() {
		return nil
	}

	if !reflect.DeepEqual(certificate.IPAddresses, validatedTemplate.IPAddresses) {
		return nil
	}

	if !reflect.DeepEqual(certificate.DNSNames, validatedTemplate.DNSNames) {
		return nil
	}

	return certificate.Raw
}

// createValidatedHTTPCertificateTemplate validates a CSR and creates a certificate template.
func createValidatedHTTPCertificateTemplate(
	owner types.NamespacedName,
	namer name.Namer,
	tls commonv1.TLSOptions,
	controllerSANs []commonv1.SubjectAlternativeName,
	svcs []corev1.Service,
	csr *x509.CertificateRequest,
	certValidity time.Duration,
) *ValidatedCertificateTemplate {
	defaultSuffixes := strings.Join(namer.DefaultSuffixes, "-")
	shortName := owner.Name + "-" + defaultSuffixes + "-" + string(HTTPCAType)
	cnNameParts := []string{
		shortName,
		owner.Namespace,
	}
	cnNameParts = append(cnNameParts, namer.DefaultSuffixes...)
	// add .local to the certificate name to avoid issuing certificates signed for .es by default
	cnNameParts = append(cnNameParts, "local")

	certCommonName := strings.Join(cnNameParts, ".")

	dnsNames := []string{
		certCommonName, // eg. clusterName-es-http.default.es.local
		shortName,      // eg. clusterName-es-http
	}
	var ipAddresses []net.IP

	for _, svc := range svcs {
		dnsNames = append(dnsNames, k8s.GetServiceDNSName(svc)...)
		ipAddresses = append(ipAddresses, k8s.GetServiceIPAddresses(svc)...)
	}

	if selfSignedCerts := tls.SelfSignedCertificate; selfSignedCerts != nil {
		for _, san := range selfSignedCerts.SubjectAlternativeNames {
			if san.DNS != "" {
				dnsNames = append(dnsNames, san.DNS)
			}
			if san.IP != "" {
				ipAddresses = append(ipAddresses, netutil.IPToRFCForm(net.ParseIP(san.IP)))
			}
		}
	}

	for _, san := range controllerSANs {
		if san.DNS != "" {
			dnsNames = append(dnsNames, san.DNS)
		}
		if san.IP != "" {
			ipAddresses = append(ipAddresses, netutil.IPToRFCForm(net.ParseIP(san.IP)))
		}
	}

	certificateTemplate := ValidatedCertificateTemplate(x509.Certificate{
		Subject: pkix.Name{
			CommonName:         certCommonName,
			OrganizationalUnit: []string{owner.Name},
		},

		DNSNames:    dnsNames,
		IPAddresses: ipAddresses,

		NotBefore: time.Now().Add(-10 * time.Minute),
		NotAfter:  time.Now().Add(certValidity),

		PublicKeyAlgorithm: csr.PublicKeyAlgorithm,
		PublicKey:          csr.PublicKey,

		Signature:          csr.Signature,
		SignatureAlgorithm: csr.SignatureAlgorithm,

		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	})

	return &certificateTemplate
}
