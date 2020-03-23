// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"crypto/rsa"

	"github.com/pkg/errors"
)

// PrivateMatchesPublicKey returns true if the public and private keys correspond to each other.
func PrivateMatchesPublicKey(publicKey interface{}, privateKey rsa.PrivateKey) bool {
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
