// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io/ioutil"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
)

// readPrivateKey reads the private key stored at the given path.
func readPrivateKey(path string) (*rsa.PrivateKey, error) {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(bytes))
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing the key")
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return privateKey, nil
}

// createAndStorePrivateKey creates a private key and writes it at the given path.
func createAndStorePrivateKey(path string) (*rsa.PrivateKey, error) {
	privateKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	pemKeyBytes := certificates.EncodePEMPrivateKey(*privateKey)
	if err := ioutil.WriteFile(path, pemKeyBytes, 0644); err != nil {
		return nil, err
	}
	return privateKey, nil
}

// readPrivateKey reads the CSR stored at the given path.
func readCSR(path string) (*x509.CertificateRequest, error) {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificateRequest(bytes)
}

// createAndStorePrivateKey creates an empty CSR from the given private key.
func createCSR(privateKey *rsa.PrivateKey) ([]byte, error) {
	return x509.CreateCertificateRequest(cryptorand.Reader, &x509.CertificateRequest{}, privateKey)
}

// readPrivateKey reads the certificate stored at the given path (pem format).
func readCert(path string) ([]*x509.Certificate, error) {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return certificates.ParsePEMCerts(bytes)
}
