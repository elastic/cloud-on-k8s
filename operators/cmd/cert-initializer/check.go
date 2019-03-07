// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"crypto/rsa"
	"crypto/x509"
	"errors"

	"github.com/elastic/k8s-operators/operators/pkg/utils/fs"
)

// checkExistingOnDisk reads the private key, csr and certificate on disk,
// and checks their validity.
func checkExistingOnDisk(config Config) bool {
	// retrieve private key that may already exist from a previous start
	privateKey, err := readPrivateKey(config.PrivateKeyPath)
	if err != nil {
		log.Info("No private key found on disk, will create one", "reason", err)
		return false
	}

	// retrieve CSR and cert that may already exist from a previous start
	csr, err := readCSR(config.CSRPath)
	if err != nil {
		log.Info("No CSR found on disk yet", "reason", err)
		return false
	}
	cert, err := readCert(config.CertPath)
	if err != nil {
		log.Info("No certificate found on disk yet", "reason", err)
		return false
	}

	// check private key matches CSR and cert
	if !privateKeyMatchesCSR(*privateKey, *csr) {
		log.Info("Private key does not match CSR, will recreate one")
		return false
	}
	if !privateKeyMatchesCerts(*privateKey, cert) {
		log.Info("Private key does not match certificate, will recreate one")
		return false
	}

	return true
}

// privateKeyMatchesCerts returns true if one of the certs public key matches the privateKey.
func privateKeyMatchesCerts(privateKey rsa.PrivateKey, certs []*x509.Certificate) bool {
	if len(certs) == 0 {
		log.Info("No certificates found")
		return false
	}
	for _, c := range certs {
		if privateMatchesPublicKey(c.PublicKey, privateKey) {
			return true
		}
	}
	return false
}

// privateKeyMatchesCerts returns true if the csr public key matches the privateKey.
func privateKeyMatchesCSR(privateKey rsa.PrivateKey, csr x509.CertificateRequest) bool {
	if err := csr.CheckSignature(); err != nil {
		log.Error(err, "Invalid CSR signature")
		return false
	}
	return privateMatchesPublicKey(csr.PublicKey, privateKey)
}

// privateKeyMatchesCerts returns true if the public and private keys correspond to each other.
func privateMatchesPublicKey(publicKey interface{}, privateKey rsa.PrivateKey) bool {
	pubKey, ok := publicKey.(*rsa.PublicKey)
	if !ok {
		log.Error(errors.New("Public key is not an RSA public key"), "")
		return false
	}
	// check that public and private keys share the same modulus and exponent
	if pubKey.N.Cmp(privateKey.N) != 0 || pubKey.E != privateKey.E {
		return false
	}
	return true
}

// watchForCertUpdate watches for changes on the cert file until it becomes valid.
func watchForCertUpdate(config Config) error {
	// on each change to the cert, check cert, csr and private key
	onEvent := func(files fs.FilesContent) (stop bool, err error) {
		if checkExistingOnDisk(config) {
			// we're good to go!
			return true, nil
		}
		return false, nil
	}
	watcher, err := fs.NewFileWatcher(config.CertPath, onEvent)
	if err != nil {
		return err
	}
	return watcher.Run()
}
