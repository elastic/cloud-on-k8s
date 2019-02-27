// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"bytes"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
)

const (
	// CAFileName is used for the CA Certificates inside a secret
	CAFileName = "ca.pem"
	// DefaultCAValidity makes new CA default to a 1 year expiration
	DefaultCAValidity = 365 * 24 * time.Hour
)

var (
	// SerialNumberLimit is the maximum number used as a certificate serial number
	SerialNumberLimit = new(big.Int).Lsh(big.NewInt(1), 128)
)

// Ca is a simple certificate authority
type Ca struct {
	// privateKey is the CA private key
	privateKey *rsa.PrivateKey
	// Cert is the certificate used to issue new certificates
	Cert *x509.Certificate
}

// ValidatedCertificateTemplate is a type alias used to convey that the certificate template has been validated and
// should be considered trusted.
type ValidatedCertificateTemplate x509.Certificate

// NewCa returns a ca with the given private key and cert
func NewCa(privateKey *rsa.PrivateKey, cert *x509.Certificate) *Ca {
	return &Ca{
		privateKey: privateKey,
		Cert:       cert,
	}
}

// CABuilderOptions are options to build a self-signed CA
type CABuilderOptions struct {
	CommonName string
	PrivateKey *rsa.PrivateKey
	ExpireIn   *time.Duration
}

// NewSelfSignedCa creates a self-signed CA according to the given options
func NewSelfSignedCa(options CABuilderOptions) (*Ca, error) {
	// generate a serial number
	serial, err := cryptorand.Int(cryptorand.Reader, SerialNumberLimit)
	if err != nil {
		return nil, err
	}

	privateKey := options.PrivateKey
	if privateKey == nil {
		privateKey, err = rsa.GenerateKey(cryptorand.Reader, 2048)
		if err != nil {
			return nil, errors.Wrap(err, "unable to generate the private key")
		}
	}

	notAfter := time.Now().Add(DefaultCAValidity)
	if options.ExpireIn != nil {
		notAfter = time.Now().Add(*options.ExpireIn)
	}

	certificateTemplate := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:         options.CommonName,
			OrganizationalUnit: []string{rand.String(16)},
		},
		NotBefore:             time.Now().Add(-1 * time.Minute),
		NotAfter:              notAfter,
		SignatureAlgorithm:    x509.SHA256WithRSA,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	certData, err := x509.CreateCertificate(cryptorand.Reader, &certificateTemplate, &certificateTemplate, privateKey.Public(), privateKey)
	if err != nil {
		return nil, err
	}

	cert, err := x509.ParseCertificate(certData)
	if err != nil {
		return nil, err
	}

	return &Ca{
		privateKey: privateKey,
		Cert:       cert,
	}, nil

}

// CreateCertificate signs and creates a new certificate for a validated template.
func (c *Ca) CreateCertificate(
	validatedCertificateTemplate ValidatedCertificateTemplate,
) ([]byte, error) {
	// generate a serial number
	serial, err := cryptorand.Int(cryptorand.Reader, SerialNumberLimit)
	if err != nil {
		return nil, errors.Wrap(err, "unable to generate serial number for new certificate")
	}
	validatedCertificateTemplate.SerialNumber = serial
	validatedCertificateTemplate.Issuer = c.Cert.Issuer

	certTemplate := x509.Certificate(validatedCertificateTemplate)

	certData, err := x509.CreateCertificate(
		cryptorand.Reader,
		&certTemplate,
		c.Cert,
		validatedCertificateTemplate.PublicKey,
		c.privateKey,
	)

	return certData, err
}

// ReconcilePublicCertsSecret ensures that a secret containing
// the CA certificate referenced with objectKey exists.
func (c *Ca) ReconcilePublicCertsSecret(
	cl k8s.Client,
	objectKey types.NamespacedName,
	owner metav1.Object,
	scheme *runtime.Scheme,
) error {
	// TODO: how to do rotation of certs here? cross signing possible, likely not.
	expectedCaKeyBytes := EncodePEMCert(c.Cert.Raw)

	clusterCASecret := corev1.Secret{
		ObjectMeta: k8s.ToObjectMeta(objectKey),
		Data: map[string][]byte{
			CAFileName: expectedCaKeyBytes,
		},
	}

	reconciled := &corev1.Secret{}
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     cl,
		Scheme:     scheme,
		Owner:      owner,
		Expected:   &clusterCASecret,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			// if Data is nil, create it in case we're starting with a poorly initialized secret
			if reconciled.Data == nil {
				reconciled.Data = make(map[string][]byte)
			}
			caKey, ok := reconciled.Data[CAFileName]
			return !ok || !bytes.Equal(caKey, expectedCaKeyBytes)

		},
		UpdateReconciled: func() {
			reconciled.Data[CAFileName] = expectedCaKeyBytes
		},
	})

}
