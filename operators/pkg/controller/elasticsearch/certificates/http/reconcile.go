// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package http

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net"
	"reflect"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	netutil "github.com/elastic/cloud-on-k8s/operators/pkg/utils/net"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("http")
)

// ReconcileHTTPCertificates reconciles the internal resources for the HTTP certificate.
func ReconcileHTTPCertificates(
	c k8s.Client,
	scheme *runtime.Scheme,
	watches watches.DynamicWatches,
	es v1alpha1.Elasticsearch,
	ca *certificates.CA,
	services []corev1.Service,
	rotationParams certificates.RotationParams,
) (*CertificatesSecret, error) {
	customCertificates, err := GetCustomCertificates(c, es)
	if err != nil {
		return nil, err
	}

	if err := reconcileDynamicWatches(watches, es); err != nil {
		return nil, err
	}

	internalCerts, err := reconcileHTTPInternalCertificatesSecret(
		c, scheme, es, services, customCertificates, ca, rotationParams,
	)
	if err != nil {
		return nil, err
	}

	return internalCerts, nil
}

// reconcileHTTPInternalCertificatesSecret ensures that the internal HTTP certificate secret has the correct content.
func reconcileHTTPInternalCertificatesSecret(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
	svcs []corev1.Service,
	customCertificates *CertificatesSecret,
	ca *certificates.CA,
	rotationParams certificates.RotationParams,
) (*CertificatesSecret, error) {
	secret := corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      name.HTTPCertsInternalSecretName(es.Name),
		},
	}

	shouldCreateSecret := false
	if err := c.Get(k8s.ExtractNamespacedName(&secret), &secret); err != nil && !apierrors.IsNotFound(err) {
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
	labels := label.NewLabels(k8s.ExtractNamespacedName(&es))
	for k, v := range labels {
		if current, ok := secret.Labels[k]; !ok || current != v {
			secret.Labels[k] = v
			needsUpdate = true
		}
	}

	if err := controllerutil.SetControllerReference(&es, &secret, scheme); err != nil {
		return nil, err
	}

	// a placeholder secret may have nil entries, create them if needed
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}

	if customCertificates != nil {
		if !reflect.DeepEqual(secret.Data, customCertificates.Data) {
			needsUpdate = true
			secret.Data = customCertificates.Data
		}
	} else {
		selfSignedNeedsUpdate, err := ensureInternalSelfSignedCertificateSecretContents(
			&secret, es, svcs, ca, rotationParams,
		)
		if err != nil {
			return nil, err
		}
		needsUpdate = needsUpdate || selfSignedNeedsUpdate
	}

	if needsUpdate {
		if shouldCreateSecret {
			log.Info("Creating HTTP internal certificate secret", "namespace", secret.Namespace, "secret_name", secret.Name)
			if err := c.Create(&secret); err != nil {
				return nil, err
			}
		} else {
			log.Info("Updating HTTP internal certificate secret", "namespace", secret.Namespace, "secret_name", secret.Name)
			if err := c.Update(&secret); err != nil {
				return nil, err
			}
		}
	}

	result := CertificatesSecret(secret)
	return &result, nil
}

// ensureInternalSelfSignedCertificateSecretContents ensures that contents of a secret containing self-signed
// certificates is valid. The provided secret is updated in-place.
//
// Returns true if the secret was changed.
func ensureInternalSelfSignedCertificateSecretContents(
	secret *corev1.Secret,
	es v1alpha1.Elasticsearch,
	svcs []corev1.Service,
	ca *certificates.CA,
	rotationParam certificates.RotationParams,
) (bool, error) {
	secretWasChanged := false

	// verify that the secret contains a parsable private key, create if it does not exist
	var privateKey *rsa.PrivateKey
	needsNewPrivateKey := true
	if privateKeyData, ok := secret.Data[certificates.KeyFileName]; ok {
		storedPrivateKey, err := certificates.ParsePEMPrivateKey(privateKeyData)
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
		secret.Data[certificates.KeyFileName] = certificates.EncodePEMPrivateKey(*privateKey)
	}

	// check if the existing cert should be re-issued
	if shouldIssueNewHTTPCertificate(es, secret, svcs, ca, rotationParam.RotateBefore) {
		log.Info(
			"Issuing new HTTP certificate",
			"namespace", secret.Namespace,
			"secret_name", secret.Name,
			"elasticsearch_name", es.Name,
			"elasticsearch_namespace", es.Namespace,
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
		validatedCertificateTemplate, err := createValidatedHTTPCertificateTemplate(
			es, svcs, parsedCSR, rotationParam.Validity,
		)
		if err != nil {
			return secretWasChanged, err
		}
		// sign the certificate
		certData, err := ca.CreateCertificate(*validatedCertificateTemplate)
		if err != nil {
			return secretWasChanged, err
		}

		secretWasChanged = true
		// store certificate and signed certificate in a secret mounted into the pod
		secret.Data[certificates.CertFileName] = certificates.EncodePEMCert(certData, ca.Cert.Raw)
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
	es v1alpha1.Elasticsearch,
	secret *corev1.Secret,
	svcs []corev1.Service,
	ca *certificates.CA,
	certReconcileBefore time.Duration,
) bool {
	validatedTemplate, err := createValidatedHTTPCertificateTemplate(
		es, svcs, &x509.CertificateRequest{}, certReconcileBefore,
	)
	if err != nil {
		return true
	}

	var certificate *x509.Certificate

	if certData, ok := secret.Data[certificates.CertFileName]; !ok {
		return true
	} else {
		certs, err := certificates.ParsePEMCerts(certData)
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
			"elasticsearch_name", es.Name,
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
	es v1alpha1.Elasticsearch,
	svcs []corev1.Service,
	csr *x509.CertificateRequest,
	certValidity time.Duration,
) (*certificates.ValidatedCertificateTemplate, error) {
	// add .cluster.local to the certificate name to avoid issuing certificates signed for .es by default
	certCommonName := fmt.Sprintf("%s.%s.es.cluster.local", es.Name, es.Namespace)

	dnsNames := []string{
		certCommonName,
	}
	var ipAddresses []net.IP

	for _, svc := range svcs {
		dnsNames = append(dnsNames, k8s.GetServiceDNSName(svc))
	}

	if selfSignedCerts := es.Spec.HTTP.TLS.SelfSignedCertificate; selfSignedCerts != nil {
		for _, san := range selfSignedCerts.SubjectAlternativeNames {
			if san.DNS != "" {
				dnsNames = append(dnsNames, san.DNS)
			}
			if san.IP != "" {
				ipAddresses = append(ipAddresses, netutil.MaybeIPTo4(net.ParseIP(san.IP)))
			}
		}
	}

	certificateTemplate := certificates.ValidatedCertificateTemplate(x509.Certificate{
		Subject: pkix.Name{
			CommonName:         certCommonName,
			OrganizationalUnit: []string{es.Name},
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

	return &certificateTemplate, nil
}
