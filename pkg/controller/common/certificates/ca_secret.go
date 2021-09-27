// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"fmt"

	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

// ParseCustomCASecret checks that mandatory fields are present and returns a CA struct.
// It does not check that the public key matches the private key.
// Legacy tls.* keys are still supported while the expected default keys are ca.crt and ca.key.
func ParseCustomCASecret(s corev1.Secret) (*CA, error) {
	keyFileName := CAKeyFileName
	crtFileName := CAFileName
	// For backwards compatibility we support both tls.* and the newer ca.* keys in the secret
	_, legacyKeyExists := s.Data[KeyFileName]
	_, legacyCrtExists := s.Data[CertFileName]
	_, keyExists := s.Data[keyFileName]
	_, crtExists := s.Data[crtFileName]
	if (legacyKeyExists || legacyCrtExists) && (keyExists || crtExists) {
		return nil, fmt.Errorf("both tls.* keys and ca.* keys exist in secret %s/%s, this is likely a configuration error", s.Namespace, s.Name)
	}
	if legacyKeyExists && legacyCrtExists {
		keyFileName = KeyFileName
		crtFileName = CertFileName
	}
	return parseCAFromSecret(s, keyFileName, crtFileName)
}

// parseCAFromSecret internal helper func to retrieve and parse a CA stored at the given keys in a Secret.
func parseCAFromSecret(s corev1.Secret, keyFileName string, crtFileName string) (*CA, error) {
	// Validate private key
	key, exist := s.Data[keyFileName]
	if !exist {
		return nil, pkgerrors.Errorf("can't find private key %s in %s/%s", keyFileName, s.Namespace, s.Name)
	}
	privateKey, err := ParsePEMPrivateKey(key)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "can't parse private key %s in %s/%s", keyFileName, s.Namespace, s.Name)
	}
	// Validate CA certificate
	cert, exist := s.Data[crtFileName]
	if !exist {
		return nil, pkgerrors.Errorf("can't find certificate %s in %s/%s", crtFileName, s.Namespace, s.Name)
	}
	pubKeys, err := ParsePEMCerts(cert)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "can't parse CA certificate %s in %s/%s", crtFileName, s.Namespace, s.Name)
	}
	if len(pubKeys) != 1 {
		return nil, pkgerrors.Errorf("only expected one PEM formated CA certificate in %s/%s", s.Namespace, s.Name)
	}
	return NewCA(privateKey, pubKeys[0]), nil
}
