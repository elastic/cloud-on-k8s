// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

// ParseCustomCASecret checks that mandatory fields are present and returns a CA struct.
// It does not check that the public key matches the private key.
func ParseCustomCASecret(s corev1.Secret) (*CA, error) {
	// Validate private key
	key, exist := s.Data[KeyFileName]
	if !exist {
		return nil, pkgerrors.Errorf("can't find private key %s in %s/%s", KeyFileName, s.Namespace, s.Name)
	}
	privateKey, err := ParsePEMPrivateKey(key)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "can't parse private key %s in %s/%s", KeyFileName, s.Namespace, s.Name)
	}
	// Validate CA certificate
	cert, exist := s.Data[CertFileName]
	if !exist {
		return nil, pkgerrors.Errorf("can't find certificate %s in %s/%s", CertFileName, s.Namespace, s.Name)
	}
	pubKeys, err := ParsePEMCerts(cert)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "can't parse CA certificate %s in %s/%s", CertFileName, s.Namespace, s.Name)
	}
	if len(pubKeys) != 1 {
		return nil, pkgerrors.Errorf("only expected one PEM formated CA certificate in %s/%s", s.Namespace, s.Name)
	}
	return NewCA(privateKey, pubKeys[0]), nil
}
