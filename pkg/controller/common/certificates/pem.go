// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

var ErrEncryptedPrivateKey = errors.New("encrypted private key")

const (
	ecPrivateKeyType    = "EC PRIVATE KEY"
	pkcs1PrivateKeyType = "RSA PRIVATE KEY"
	pkcs8PrivateKeyType = "PRIVATE KEY"
)

// ParsePEMCerts returns a list of certificates from the given PEM certs data
// Based on the code of x509.CertPool.AppendCertsFromPEM (https://golang.org/src/crypto/x509/cert_pool.go)
// We don't rely on x509.CertPool.AppendCertsFromPEM directly here since it returns an interface from which
// we cannot extract the actual certificates if we need to compare them.
func ParsePEMCerts(pemData []byte) ([]*x509.Certificate, error) {
	certs := []*x509.Certificate{}
	for len(pemData) > 0 {
		var block *pem.Block
		block, pemData = pem.Decode(pemData)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		certs = append(certs, cert)
	}
	return certs, nil
}

// EncodePEMCert encodes the given certificate blocks as a PEM certificate
func EncodePEMCert(certBlocks ...[]byte) []byte {
	var buf bytes.Buffer
	for _, block := range certBlocks {
		_, _ = buf.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: block}))
	}
	return buf.Bytes()
}

// EncodePEMPrivateKey encodes the given private key in the PEM format
func EncodePEMPrivateKey(privateKey crypto.Signer) ([]byte, error) {
	pemBlock, err := pemBlockForKey(privateKey)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(pemBlock), nil
}

func pemBlockForKey(privateKey interface{}) (*pem.Block, error) {
	switch k := privateKey.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: pkcs1PrivateKeyType, Bytes: x509.MarshalPKCS1PrivateKey(k)}, nil
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, err
		}
		return &pem.Block{Type: ecPrivateKeyType, Bytes: b}, nil
	default:
		// attempt PKCS#8 format
		b, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			return nil, err
		}
		return &pem.Block{Type: pkcs8PrivateKeyType, Bytes: b}, nil
	}
}

// ParsePEMPrivateKey parses the given private key in the PEM format
// ErrEncryptedPrivateKey is returned as an error if the private key is encrypted.
func ParsePEMPrivateKey(pemData []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing private key")
	}

	switch {
	case x509.IsEncryptedPEMBlock(block): //nolint:staticcheck
		// Private key is encrypted, do not attempt to parse it
		return nil, ErrEncryptedPrivateKey
	case block.Type == pkcs8PrivateKeyType:
		return parsePKCS8PrivateKey(block.Bytes)
	case block.Type == pkcs1PrivateKeyType && len(block.Headers) == 0:
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case block.Type == ecPrivateKeyType:
		return x509.ParseECPrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported private key type %q", block.Type)
	}
}

func parsePKCS8PrivateKey(block []byte) (*rsa.PrivateKey, error) {
	key, err := x509.ParsePKCS8PrivateKey(block)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse private key")
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.Errorf("expected an RSA private key but got %t", key)
	}

	return rsaKey, nil
}

// GetPrimaryCertificate returns the primary certificate (i.e. the actual subject, not a CA or intermediate) from a PEM certificate chain
func GetPrimaryCertificate(pemBytes []byte) (*x509.Certificate, error) {
	parsedCerts, err := ParsePEMCerts(pemBytes)
	if err != nil {
		return nil, err
	}
	// the primary certificate should always come first, see:
	// http://tools.ietf.org/html/rfc4346#section-7.4.2
	if len(parsedCerts) < 1 {
		return nil, errors.New("Expected at least one certificate")
	}
	return parsedCerts[0], nil
}

// PrivateMatchesPublicKey returns true if the public and private keys correspond to each other.
func PrivateMatchesPublicKey(ctx context.Context, publicKey crypto.PublicKey, privateKey crypto.Signer) bool {
	switch k := publicKey.(type) {
	case *rsa.PublicKey:
		return k.Equal(privateKey.Public())
	case *ecdsa.PublicKey:
		return k.Equal(privateKey.Public())
	default:
		ulog.FromContext(ctx).Error(fmt.Errorf("unsupported public key type: %T", publicKey), "")
		return false
	}
}

// GetCompatiblePrivateKey returns a PEM encoded private key iff the CA and the key have the same underlying type.
func GetCompatiblePrivateKey(ctx context.Context, caPrivateKey crypto.Signer, secret *corev1.Secret, fileName string) crypto.Signer {
	if certPrivateKeyData, ok := secret.Data[fileName]; ok {
		log := ulog.FromContext(ctx)
		certPrivateKey, err := ParsePEMPrivateKey(certPrivateKeyData)
		if err != nil {
			log.Error(err, "Unable to parse stored private key", "namespace", secret.Namespace, "secret_name", secret.Name, "cert_key_filename", fileName)
			return nil
		}
		if caPrivateKey == nil || certPrivateKey == nil {
			return nil
		}
		if reflect.TypeOf(caPrivateKey) != reflect.TypeOf(certPrivateKey) {
			log.Info(
				"CA and cert private key do not share the same implementation",
				"namespace", secret.Namespace,
				"secret_name", secret.Name,
				"cert_key_filename", fileName,
				"ca_type", reflect.TypeOf(caPrivateKey),
				"cert_type", reflect.TypeOf(certPrivateKey),
			)
			return nil
		}
		return certPrivateKey
	}
	return nil
}

// NewPrivateKey generates a new private key using the same implementation than the CA.
func NewPrivateKey(caSigner crypto.Signer) (crypto.Signer, error) {
	switch k := caSigner.(type) {
	case *rsa.PrivateKey:
		return rsa.GenerateKey(cryptorand.Reader, 2048)
	case *ecdsa.PrivateKey:
		// re-use the same curve
		return ecdsa.GenerateKey(k.PublicKey.Curve, cryptorand.Reader)
	default:
		return nil, fmt.Errorf("unsupported CA private key type: %T", caSigner)
	}
}
