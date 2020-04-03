// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
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

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	netutil "github.com/elastic/cloud-on-k8s/pkg/utils/net"
)

// ReconcilePublicHTTPCerts reconciles the Secret containing the HTTP Certificate currently in use, and the CA of
// the certificate if available.
func (r Reconciler) ReconcilePublicHTTPCerts(internalCerts *CertificatesSecret) error {
	nsn := PublicCertsSecretRef(r.Namer, k8s.ExtractNamespacedName(r.Object))
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

	_, err := reconciler.ReconcileSecret(r.K8sClient, expected, r.Object)
	return err
}

// ReconcileInternalHTTPCerts reconciles the internal resources for the HTTP certificate.
func (r Reconciler) ReconcileInternalHTTPCerts(ca *CA) (*CertificatesSecret, error) {
	ownerNSN := k8s.ExtractNamespacedName(r.Object)
	customCertificates, err := GetCustomCertificates(r.K8sClient, ownerNSN, r.TLSOptions)
	if err != nil {
		return nil, err
	}

	if err := reconcileDynamicWatches(r.DynamicWatches, ownerNSN, r.Namer, r.TLSOptions); err != nil {
		return nil, err
	}

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.Object.GetNamespace(),
			Name:      InternalCertsSecretName(r.Namer, r.Object.GetName()),
		},
	}

	shouldCreateSecret := false
	if err := r.K8sClient.Get(k8s.ExtractNamespacedName(&secret), &secret); err != nil && !apierrors.IsNotFound(err) {
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

	if err := controllerutil.SetControllerReference(r.Object, &secret, scheme.Scheme); err != nil {
		return nil, err
	}

	// a placeholder secret may have nil entries, create them if needed
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}

	if customCertificates != nil {
		if err := customCertificates.Validate(); err != nil {
			return nil, err
		}
		expectedSecretData := make(map[string][]byte)
		expectedSecretData[CertFileName] = customCertificates.CertPem()
		expectedSecretData[KeyFileName] = customCertificates.KeyPem()
		if caPem := customCertificates.CAPem(); len(caPem) > 0 {
			expectedSecretData[CAFileName] = caPem
		} else {
			// Ensure that the CA certificate is never empty, otherwise Elasticsearch is not able to reload the certificates.
			// Default to our self-signed (useless) CA if none is provided by the user.
			// See https://github.com/elastic/cloud-on-k8s/issues/2243
			expectedSecretData[CAFileName] = EncodePEMCert(ca.Cert.Raw)
		}

		if !reflect.DeepEqual(secret.Data, expectedSecretData) {
			needsUpdate = true
			secret.Data = expectedSecretData
		}
	} else {
		selfSignedNeedsUpdate, err := ensureInternalSelfSignedCertificateSecretContents(
			&secret, ownerNSN, r.Namer, r.TLSOptions, r.Services, ca, r.CertRotation,
		)
		if err != nil {
			return nil, err
		}
		needsUpdate = needsUpdate || selfSignedNeedsUpdate
	}

	if needsUpdate {
		if shouldCreateSecret {
			log.Info("Creating HTTP internal certificate secret", "namespace", secret.Namespace, "secret_name", secret.Name)
			if err := r.K8sClient.Create(&secret); err != nil {
				return nil, err
			}
		} else {
			log.Info("Updating HTTP internal certificate secret", "namespace", secret.Namespace, "secret_name", secret.Name)
			if err := r.K8sClient.Update(&secret); err != nil {
				return nil, err
			}
		}
	}

	internalCerts := CertificatesSecret(secret)
	return &internalCerts, nil
}

// ensureInternalSelfSignedCertificateSecretContents ensures that contents of a secret containing self-signed
// certificates is valid. The provided secret is updated in-place.
//
// Returns true if the secret was changed.
func ensureInternalSelfSignedCertificateSecretContents(
	secret *corev1.Secret,
	owner types.NamespacedName,
	namer name.Namer,
	tls commonv1.TLSOptions,
	svcs []corev1.Service,
	ca *CA,
	rotationParam RotationParams,
) (bool, error) {
	secretWasChanged := false

	// verify that the secret contains a parsable private key, create if it does not exist
	var privateKey *rsa.PrivateKey
	needsNewPrivateKey := true
	if privateKeyData, ok := secret.Data[KeyFileName]; ok {
		storedPrivateKey, err := ParsePEMPrivateKey(privateKeyData)
		if err != nil {
			log.Error(err, "Unable to parse stored private key", "namespace", secret.Namespace, "secret_name", secret.Name)
		} else {
			needsNewPrivateKey = false
			privateKey = storedPrivateKey
		}
	}

	// if we need a new private key, generate it
	if needsNewPrivateKey {
		generatedPrivateKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
		if err != nil {
			return secretWasChanged, err
		}

		privateKey = generatedPrivateKey
		secretWasChanged = true
		secret.Data[KeyFileName] = EncodePEMPrivateKey(*privateKey)
	}

	// check if the existing cert should be re-issued
	if shouldIssueNewHTTPCertificate(owner, namer, tls, secret, svcs, ca, rotationParam.RotateBefore) {
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
			owner, namer, tls, svcs, parsedCSR, rotationParam.Validity,
		)
		// sign the certificate
		certData, err := ca.CreateCertificate(*validatedCertificateTemplate)
		if err != nil {
			return secretWasChanged, err
		}

		secretWasChanged = true
		// store certificate and signed certificate in a secret mounted into the pod
		secret.Data[CAFileName] = EncodePEMCert(ca.Cert.Raw)
		secret.Data[CertFileName] = EncodePEMCert(certData, ca.Cert.Raw)
	}

	return secretWasChanged, nil
}

// shouldIssueNewHTTPCertificate returns true if we should issue a new HTTP certificate.
//
// Reasons for reissuing a certificate:
//
//   - no certificate yet
//   - certificate has the wrong format
//   - certificate is invalid according to the CA or expired
//   - certificate SAN and IP does not match the expected ones
func shouldIssueNewHTTPCertificate(
	owner types.NamespacedName,
	namer name.Namer,
	tls commonv1.TLSOptions,
	secret *corev1.Secret,
	svcs []corev1.Service,
	ca *CA,
	certReconcileBefore time.Duration,
) bool {
	validatedTemplate := createValidatedHTTPCertificateTemplate(
		owner, namer, tls, svcs, &x509.CertificateRequest{}, certReconcileBefore,
	)

	var certificate *x509.Certificate

	certData, ok := secret.Data[CertFileName]
	if !ok {
		return true
	}
	certs, err := ParsePEMCerts(certData)
	if err != nil {
		log.Error(err, "Invalid certificate data found, issuing new certificate", "namespace", secret.Namespace, "secret_name", secret.Name)
		return true
	}

	// look for the certificate based on the CommonName
	for _, c := range certs {
		if c.Subject.CommonName == validatedTemplate.Subject.CommonName {
			certificate = c
			break
		}
	}

	if certificate == nil {
		return true
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
		return true
	}

	if time.Now().After(certificate.NotAfter.Add(-certReconcileBefore)) {
		log.Info("Certificate soon to expire, should issue new", "namespace", secret.Namespace, "secret_name", secret.Name)
		return true
	}

	if certificate.Subject.String() != validatedTemplate.Subject.String() {
		return true
	}

	if !reflect.DeepEqual(certificate.IPAddresses, validatedTemplate.IPAddresses) {
		return true
	}

	if !reflect.DeepEqual(certificate.DNSNames, validatedTemplate.DNSNames) {
		return true
	}

	return false
}

// createValidatedHTTPCertificateTemplate validates a CSR and creates a certificate template.
func createValidatedHTTPCertificateTemplate(
	owner types.NamespacedName,
	namer name.Namer,
	tls commonv1.TLSOptions,
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
	}

	if selfSignedCerts := tls.SelfSignedCertificate; selfSignedCerts != nil {
		for _, san := range selfSignedCerts.SubjectAlternativeNames {
			if san.DNS != "" {
				dnsNames = append(dnsNames, san.DNS)
			}
			if san.IP != "" {
				ipAddresses = append(ipAddresses, netutil.MaybeIPTo4(net.ParseIP(san.IP)))
			}
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
