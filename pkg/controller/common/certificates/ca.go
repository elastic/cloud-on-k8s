// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"context"
	"crypto"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var (
	// SerialNumberLimit is the maximum number used as a certificate serial number
	SerialNumberLimit = new(big.Int).Lsh(big.NewInt(1), 128)
)

// CA is a simple certificate authority
type CA struct {
	// PrivateKey is the CA private key
	PrivateKey crypto.Signer
	// Cert is the certificate used to issue new certificates
	Cert *x509.Certificate
}

// ValidatedCertificateTemplate is a type alias used to convey that the certificate template has been validated and
// should be considered trusted.
type ValidatedCertificateTemplate x509.Certificate

// NewCA returns a ca with the given private key and cert
func NewCA(privateKey crypto.Signer, cert *x509.Certificate) *CA {
	return &CA{
		PrivateKey: privateKey,
		Cert:       cert,
	}
}

// CABuilderOptions are options to build a self-signed CA
type CABuilderOptions struct {
	// Subject of the CA to build.
	Subject pkix.Name
	// PrivateKey to be used for signing certificates (auto-generated if not provided).
	PrivateKey *rsa.PrivateKey
	// ExpireIn defines in how much time will the CA expire (defaults to DefaultCertValidity if not provided).
	ExpireIn *time.Duration
}

// NewSelfSignedCA creates a self-signed CA according to the given options
func NewSelfSignedCA(options CABuilderOptions) (*CA, error) {
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

	notAfter := time.Now().Add(DefaultCertValidity)
	if options.ExpireIn != nil {
		notAfter = time.Now().Add(*options.ExpireIn)
	}

	certificateTemplate := x509.Certificate{
		SerialNumber:          serial,
		Subject:               options.Subject,
		NotBefore:             time.Now().Add(-10 * time.Minute),
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

	return &CA{
		PrivateKey: privateKey,
		Cert:       cert,
	}, nil
}

// CreateCertificate signs and creates a new certificate for a validated template.
func (c *CA) CreateCertificate(
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
		c.PrivateKey,
	)

	return certData, err
}

// PublicCertsHasCACert returns true if an Elastic resource has a CA (ca.crt) in its public HTTP certs secret given its namer,
// namespace and name.
func PublicCertsHasCACert(client k8s.Client, namer name.Namer, namespace string, name string) (bool, error) {
	certsNsn := types.NamespacedName{
		Name:      PublicCertsSecretName(namer, name),
		Namespace: namespace,
	}
	var certsSecret corev1.Secret
	if err := client.Get(context.Background(), certsNsn, &certsSecret); err != nil {
		return false, err
	}
	_, ok := certsSecret.Data[CAFileName]
	return ok, nil
}
