// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystore

// Keystore format version used by this implementation.
// See: https://github.com/elastic/elasticsearch/blob/main/server/src/main/java/org/elasticsearch/common/settings/KeyStoreWrapper.java
//
// This implementation only supports ES 9.3+ (which uses keystore format V7).
// Older keystore versions (V4-V6) are not needed since the Go keystore feature
// is gated to ES 9.3+ via MinESVersion in reconciler.go.
const (
	// KeystoreVersion is the keystore format version for ES 9.0+
	// V7 uses: 210,000 KDF iterations, 256-bit AES key, little endian encoding
	KeystoreVersion = 7
)

// VersionConfig holds the cryptographic parameters for the keystore.
type VersionConfig struct {
	// KDFIterations is the number of PBKDF2 iterations
	KDFIterations int
	// CipherKeyBits is the AES key size in bits
	CipherKeyBits int
	// UseLittleEndian indicates whether to use little endian for data encoding
	UseLittleEndian bool
}

// Config is the cryptographic configuration for the keystore.
// ES 9.0+ uses V7: 210,000 KDF iterations, 256-bit AES, little endian.
var Config = VersionConfig{
	KDFIterations:   210000,
	CipherKeyBits:   256,
	UseLittleEndian: true,
}
