// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var licenseFixture = EnterpriseLicense{
	License: LicenseSpec{
		UID:                "1A3E10B3-78AD-459B-86B9-230A53B3F282",
		IssueDateInMillis:  1548115200000,
		ExpiryDateInMillis: 1561247999999,
		IssuedTo:           "Ben Bitdiddle",
		Issuer:             "Alyssa P. Hacker,",
		StartDateInMillis:  1548115200000,
		Type:               "enterprise",
		MaxInstances:       23,
		Signature:          string(signatureFixture),
	},
}

func withSignature(l EnterpriseLicense, sig []byte) EnterpriseLicense {
	l.License.Signature = string(sig)
	return l
}

func asRuntimeObjects(l EnterpriseLicense, sig []byte) []runtime.Object {
	bytes, err := json.Marshal(withSignature(l, sig))
	if err != nil {
		panic(err)
	}
	return []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "test-system",
				Name:      "test-license",
			},
			Data: map[string][]byte{
				"_": bytes,
			},
		},
	}
}

func publicKeyBytesFixture(t *testing.T) []byte {
	bytes, err := x509.MarshalPKIXPublicKey(publicKeyFixture(t))
	require.NoError(t, err)
	return bytes
}

func publicKeyFixture(t *testing.T) *rsa.PublicKey {
	key, err := x509.ParsePKCS1PrivateKey(privateKeyFixture)
	require.NoError(t, err)
	return &key.PublicKey
}

var privateKeyFixture = []byte{
	0x30, 0x82, 0x04, 0xa5, 0x02, 0x01, 0x00, 0x02,
	0x82, 0x01, 0x01, 0x00, 0xe3, 0x4d, 0xbc, 0x9b,
	0x76, 0x1e, 0x1a, 0xa4, 0x89, 0x28, 0x6e, 0x0f,
	0x7e, 0x10, 0xf2, 0x87, 0x52, 0xbc, 0x64, 0xd2,
	0x94, 0x46, 0x78, 0xf1, 0x5b, 0xd0, 0xda, 0xd3,
	0xdc, 0x56, 0x6e, 0x62, 0x6f, 0xa1, 0x9e, 0x12,
	0x65, 0x35, 0xcf, 0x92, 0x2e, 0xdd, 0xf3, 0xf2,
	0xd9, 0xf9, 0x99, 0x00, 0xa1, 0x11, 0xfd, 0x34,
	0x0b, 0x43, 0xb5, 0xb5, 0xce, 0x5f, 0xf4, 0xce,
	0x51, 0xf9, 0x35, 0x1f, 0xed, 0x94, 0xd4, 0x2c,
	0x92, 0x95, 0x75, 0x50, 0x88, 0x2f, 0x89, 0x41,
	0x79, 0x17, 0xd4, 0x99, 0x38, 0x5a, 0x3e, 0xab,
	0xa2, 0x84, 0x2c, 0x2b, 0x13, 0x94, 0x85, 0xff,
	0x2d, 0x35, 0xd7, 0x8e, 0x6e, 0x17, 0xf4, 0x64,
	0x5d, 0x93, 0x8b, 0x9a, 0x44, 0x17, 0x81, 0x48,
	0xe9, 0x38, 0xd8, 0x59, 0x63, 0xd9, 0x6b, 0xaf,
	0xad, 0x7f, 0xb2, 0x71, 0x4b, 0xd4, 0xe3, 0x6a,
	0xe7, 0x7b, 0x44, 0x9d, 0x0a, 0x19, 0x1e, 0xa4,
	0x10, 0x0b, 0x2b, 0x8d, 0x69, 0x98, 0xa4, 0x40,
	0xe2, 0xa9, 0x2e, 0x78, 0x4e, 0x88, 0xe2, 0x85,
	0x22, 0xf8, 0x78, 0xcc, 0x37, 0x6f, 0x8e, 0x14,
	0xee, 0xd8, 0x8a, 0x61, 0xe1, 0x9a, 0xac, 0x2d,
	0x83, 0x9a, 0x65, 0x96, 0x9c, 0xa5, 0xd9, 0x73,
	0x70, 0x26, 0xa7, 0x86, 0x56, 0x2e, 0xd6, 0xa2,
	0xff, 0x28, 0x38, 0x32, 0x89, 0xba, 0x1b, 0x86,
	0x0f, 0xfa, 0xfe, 0x0f, 0xa7, 0xf6, 0xc7, 0x8a,
	0x0c, 0x14, 0x8c, 0xcc, 0x60, 0x89, 0x41, 0x67,
	0x3e, 0xc1, 0xef, 0x9c, 0x29, 0x42, 0xa7, 0xe1,
	0x51, 0xf6, 0x17, 0xb1, 0x51, 0xd0, 0x76, 0xf9,
	0x39, 0x7c, 0x3e, 0xb0, 0x28, 0xe2, 0x45, 0x09,
	0xef, 0x43, 0xb6, 0x97, 0xa3, 0xb1, 0x9a, 0xee,
	0x19, 0x18, 0x95, 0x79, 0x01, 0xf9, 0x70, 0x35,
	0xed, 0x3f, 0x97, 0x75, 0xb7, 0xa0, 0x9b, 0x47,
	0x00, 0x9e, 0xda, 0x95, 0x02, 0x03, 0x01, 0x00,
	0x01, 0x02, 0x82, 0x01, 0x01, 0x00, 0xc2, 0x19,
	0xf3, 0xe1, 0x5d, 0x70, 0x3f, 0x98, 0x19, 0x77,
	0xe8, 0xe4, 0x59, 0xe6, 0xe6, 0xf4, 0x1f, 0xf6,
	0xb8, 0xb2, 0x09, 0xe4, 0x54, 0x0a, 0xe7, 0x38,
	0xe6, 0x64, 0xdc, 0x57, 0x02, 0x54, 0x14, 0xb7,
	0x92, 0x60, 0x6b, 0x05, 0x14, 0x87, 0xe4, 0x75,
	0xac, 0x87, 0xc9, 0x13, 0x97, 0x50, 0x2e, 0x3b,
	0x4a, 0x59, 0x52, 0xf5, 0x33, 0x0a, 0x59, 0x7d,
	0x1f, 0x73, 0xc0, 0x14, 0x6b, 0x05, 0x24, 0xc0,
	0x5e, 0x9e, 0xe1, 0x5d, 0xb7, 0x9c, 0x59, 0x6f,
	0x1e, 0x6a, 0x46, 0x99, 0xce, 0xf6, 0x38, 0x64,
	0xf7, 0xf7, 0x61, 0x04, 0x35, 0x23, 0x63, 0xc9,
	0x04, 0xd3, 0xef, 0x2b, 0x77, 0x33, 0x2d, 0x58,
	0x4f, 0x5e, 0x15, 0x7a, 0x95, 0x4f, 0xda, 0xad,
	0xa7, 0xbd, 0x37, 0x4b, 0x4f, 0x94, 0xa5, 0xeb,
	0x58, 0x45, 0xe0, 0x86, 0x97, 0x3e, 0xa0, 0x5e,
	0xdf, 0x04, 0xbf, 0x3f, 0x48, 0x4a, 0xa5, 0xe9,
	0x75, 0x54, 0x4e, 0xbe, 0x4b, 0xa2, 0xa9, 0xb7,
	0x90, 0xb1, 0x7e, 0x25, 0x5c, 0x92, 0x2b, 0x0b,
	0x4b, 0x0a, 0x54, 0x26, 0xd2, 0x95, 0xbd, 0x33,
	0xdb, 0x9e, 0x35, 0x5d, 0xb7, 0xd1, 0xeb, 0x64,
	0x7e, 0x83, 0x93, 0xa3, 0x80, 0x6c, 0xe6, 0x29,
	0xcd, 0x94, 0xe1, 0x07, 0x52, 0xef, 0xcc, 0xdd,
	0x7b, 0xc7, 0x0d, 0x55, 0x4d, 0xeb, 0x60, 0xff,
	0x95, 0xb3, 0x74, 0xe0, 0x2d, 0x36, 0xa1, 0x9a,
	0x46, 0x4c, 0x49, 0x32, 0x20, 0xd5, 0x37, 0x44,
	0x05, 0xdf, 0x02, 0x17, 0x33, 0x5a, 0x22, 0xf7,
	0x14, 0x96, 0x4a, 0x7d, 0xe4, 0xd6, 0xb5, 0x35,
	0xf6, 0x15, 0x44, 0x47, 0x8e, 0xbc, 0xeb, 0x3d,
	0xb7, 0xe4, 0xf9, 0x09, 0x97, 0x23, 0x13, 0x69,
	0xe3, 0x8c, 0x50, 0x92, 0x40, 0x80, 0x8d, 0x3c,
	0xe2, 0x7f, 0x1e, 0x2a, 0x44, 0x3a, 0x00, 0x94,
	0x83, 0x89, 0x8b, 0x3c, 0xf6, 0x81, 0x02, 0x81,
	0x81, 0x00, 0xf6, 0xc2, 0x6b, 0xe2, 0xd6, 0x8c,
	0x9f, 0x86, 0x94, 0xf6, 0xf6, 0x65, 0xb0, 0x0e,
	0xb9, 0xdf, 0x8e, 0xc8, 0x1b, 0x1b, 0xba, 0x26,
	0x20, 0x4c, 0x96, 0xea, 0x99, 0xb7, 0x80, 0xc5,
	0xc1, 0x57, 0x07, 0xe1, 0x7b, 0xd2, 0x81, 0x96,
	0xd8, 0x41, 0xa8, 0x1e, 0xa7, 0x2e, 0xfd, 0xa2,
	0x54, 0x41, 0x5a, 0x97, 0x1b, 0x78, 0x26, 0x34,
	0xcb, 0x7c, 0xfa, 0xad, 0x53, 0x5c, 0x58, 0x14,
	0x25, 0x16, 0x36, 0x73, 0x96, 0xe3, 0xb2, 0x85,
	0xb3, 0x77, 0xbe, 0x51, 0x85, 0xcf, 0x0e, 0x60,
	0x00, 0xb3, 0x54, 0x9a, 0x58, 0xd5, 0xdc, 0xef,
	0xb5, 0x70, 0x38, 0x24, 0x80, 0x0c, 0x7f, 0x08,
	0x1e, 0x65, 0x81, 0x62, 0xb2, 0xd9, 0xe7, 0x9a,
	0xc0, 0x99, 0x9f, 0xf3, 0x8e, 0x1f, 0x6d, 0xf4,
	0xb5, 0xeb, 0x8b, 0x1f, 0x50, 0x49, 0xd5, 0x69,
	0xe7, 0x1a, 0xbc, 0xe4, 0x63, 0x21, 0xce, 0x58,
	0x58, 0x09, 0x02, 0x81, 0x81, 0x00, 0xeb, 0xd0,
	0xcd, 0x00, 0xea, 0xf3, 0x1b, 0xfc, 0xa1, 0xf9,
	0x54, 0xd0, 0xdd, 0x6a, 0x60, 0x24, 0x6a, 0x5a,
	0x1e, 0xdd, 0xc4, 0x4d, 0xc4, 0x98, 0xf3, 0xa3,
	0x32, 0x4f, 0x8e, 0x55, 0x08, 0xf9, 0xb5, 0x97,
	0xd7, 0x98, 0x17, 0xe2, 0x32, 0x0b, 0x3d, 0xb9,
	0x55, 0x25, 0xfd, 0x07, 0x22, 0x53, 0xe5, 0x69,
	0x08, 0x74, 0x16, 0x51, 0xa8, 0xfa, 0x5c, 0xbc,
	0x92, 0x5a, 0xbe, 0x6a, 0xd6, 0x04, 0x01, 0xa7,
	0xe5, 0x2d, 0xe6, 0x5d, 0x45, 0x94, 0xdb, 0x7b,
	0xaf, 0x36, 0x26, 0xba, 0xcc, 0x28, 0x0e, 0x61,
	0x63, 0xc9, 0xae, 0x91, 0xd4, 0xfa, 0x0a, 0xb3,
	0x8a, 0xfd, 0xce, 0xaf, 0xd5, 0xd6, 0x45, 0xf5,
	0xf0, 0xfe, 0x2a, 0x3f, 0xb1, 0xd9, 0x71, 0xd0,
	0xca, 0x98, 0xd1, 0x88, 0xe8, 0x31, 0xc9, 0x33,
	0xd1, 0xf4, 0xe8, 0xd2, 0x79, 0xd3, 0xe2, 0xae,
	0x4b, 0x5d, 0xd5, 0x2a, 0x99, 0x2d, 0x02, 0x81,
	0x81, 0x00, 0xdc, 0x2d, 0x5c, 0xb0, 0x99, 0xf9,
	0xfd, 0xf9, 0xca, 0xff, 0x95, 0x99, 0xe4, 0x7f,
	0x2f, 0x51, 0x10, 0x08, 0xf3, 0x4f, 0x49, 0x48,
	0xed, 0xb7, 0x09, 0x4e, 0x29, 0x7c, 0xb8, 0x55,
	0x3c, 0x0f, 0x99, 0x03, 0x94, 0x45, 0x9f, 0xc5,
	0xe6, 0x0e, 0xa7, 0xa1, 0x3a, 0x51, 0xce, 0x94,
	0xc2, 0x93, 0x51, 0xee, 0xd4, 0xde, 0xdf, 0x50,
	0x6a, 0x65, 0x89, 0x13, 0x90, 0xf7, 0x2b, 0xcc,
	0x45, 0xcf, 0x4d, 0x24, 0xd4, 0x75, 0x35, 0x7c,
	0xe1, 0x47, 0x2e, 0x35, 0x75, 0xac, 0xec, 0x49,
	0xb3, 0x36, 0x50, 0x7e, 0x2c, 0x58, 0x1f, 0x7c,
	0x70, 0x2b, 0xc2, 0x9c, 0xa6, 0xf8, 0xff, 0x7c,
	0x52, 0x0b, 0x06, 0x68, 0xf7, 0xe7, 0x41, 0x26,
	0x2f, 0x46, 0xa4, 0x97, 0x60, 0xb0, 0x20, 0x9f,
	0xa2, 0x97, 0x9a, 0x9a, 0x85, 0x3c, 0x6c, 0x45,
	0xc3, 0xa5, 0x72, 0xf8, 0x62, 0x8f, 0xee, 0x9b,
	0x9b, 0x69, 0x02, 0x81, 0x80, 0x54, 0x77, 0xc7,
	0x66, 0xe3, 0xc1, 0xcf, 0x2d, 0x90, 0x0b, 0x52,
	0x71, 0x3a, 0x4e, 0x67, 0x3f, 0xc4, 0x04, 0xa1,
	0xf7, 0xc7, 0xe0, 0x1f, 0x62, 0xb6, 0x2a, 0xa7,
	0xd3, 0xcd, 0x64, 0xf2, 0x41, 0x17, 0xe5, 0xda,
	0xe8, 0xf4, 0xed, 0x26, 0x05, 0xd6, 0xc7, 0x33,
	0x13, 0xd7, 0x6d, 0x9d, 0xc3, 0x35, 0x72, 0x88,
	0xff, 0xa4, 0x1a, 0xfe, 0x0f, 0x27, 0xf6, 0xb7,
	0xe9, 0xdf, 0x39, 0x3f, 0x8d, 0xd1, 0xd6, 0x05,
	0x06, 0x8a, 0xf4, 0xaf, 0xfe, 0xe1, 0x1b, 0x8d,
	0xa8, 0x34, 0xf9, 0x46, 0x35, 0xb6, 0xe8, 0xf5,
	0xa8, 0x81, 0x6a, 0x65, 0x42, 0x67, 0x60, 0xe6,
	0x91, 0x81, 0x5e, 0x84, 0x97, 0x2b, 0x1a, 0x2c,
	0x87, 0xae, 0x34, 0x80, 0x8d, 0x25, 0xf2, 0xa7,
	0x0f, 0x54, 0x46, 0xd8, 0xfd, 0x34, 0x57, 0xe6,
	0x85, 0xf6, 0x7b, 0xa5, 0xfd, 0xda, 0xbd, 0x99,
	0xeb, 0x73, 0x76, 0xbd, 0xc5, 0x02, 0x81, 0x81,
	0x00, 0xb7, 0xf5, 0xbd, 0xf0, 0xde, 0x23, 0x87,
	0x17, 0xcf, 0x92, 0xda, 0x5e, 0xe3, 0x73, 0x59,
	0xcf, 0xbf, 0xfa, 0xdf, 0x37, 0x6c, 0x7e, 0x1b,
	0x5c, 0x17, 0xc7, 0x4d, 0xb8, 0xf0, 0x8f, 0x65,
	0xed, 0xe2, 0xcb, 0x25, 0x05, 0x28, 0x73, 0x8b,
	0x3a, 0xeb, 0x8c, 0x88, 0x4b, 0xfc, 0x35, 0x1c,
	0x0a, 0xd3, 0xed, 0x11, 0xc7, 0x51, 0x4c, 0xe6,
	0x8a, 0x36, 0x6d, 0x4f, 0x37, 0xf0, 0x56, 0x26,
	0x2f, 0x14, 0x84, 0x22, 0x49, 0x80, 0x45, 0xfa,
	0xd0, 0xe5, 0x98, 0x1e, 0x9a, 0x97, 0x54, 0xf6,
	0x49, 0x4f, 0xcd, 0xb7, 0xf7, 0x12, 0x16, 0x08,
	0x5e, 0xea, 0xb6, 0x9c, 0xb0, 0x0e, 0x3e, 0xb9,
	0x43, 0x23, 0x97, 0x39, 0xdc, 0x4c, 0xbf, 0xd8,
	0xe0, 0xd2, 0x26, 0x2c, 0x15, 0xe0, 0x8c, 0xd3,
	0xc1, 0xb9, 0x20, 0xff, 0xc0, 0xfd, 0xb8, 0xd9,
	0xa1, 0x72, 0x90, 0xd8, 0x25, 0x8b, 0x10, 0x2b,
	0x94,
}

var signatureFixture = []byte{
	0x41, 0x41, 0x41, 0x41, 0x41, 0x77, 0x41, 0x41,
	0x41, 0x41, 0x30, 0x31, 0x38, 0x47, 0x4c, 0x47,
	0x61, 0x52, 0x44, 0x71, 0x55, 0x45, 0x69, 0x2f,
	0x68, 0x70, 0x68, 0x68, 0x41, 0x41, 0x41, 0x42,
	0x61, 0x45, 0x31, 0x4a, 0x53, 0x55, 0x4a, 0x44,
	0x5a, 0x30, 0x74, 0x44, 0x51, 0x56, 0x46, 0x46,
	0x51, 0x54, 0x51, 0x77, 0x4d, 0x6a, 0x68, 0x74,
	0x4d, 0x31, 0x6c, 0x6c, 0x52, 0x33, 0x46, 0x54,
	0x53, 0x6b, 0x74, 0x48, 0x4e, 0x46, 0x42, 0x6d,
	0x61, 0x45, 0x52, 0x35, 0x61, 0x44, 0x46, 0x4c,
	0x4f, 0x46, 0x70, 0x4f, 0x53, 0x31, 0x56, 0x53,
	0x62, 0x6d, 0x70, 0x34, 0x56, 0x7a, 0x6c, 0x45,
	0x59, 0x54, 0x41, 0x35, 0x65, 0x46, 0x64, 0x69,
	0x62, 0x55, 0x70, 0x32, 0x62, 0x31, 0x6f, 0x30,
	0x55, 0x31, 0x70, 0x55, 0x57, 0x46, 0x42, 0x72,
	0x61, 0x54, 0x64, 0x6b, 0x4f, 0x43, 0x39, 0x4d,
	0x57, 0x69, 0x74, 0x61, 0x61, 0x30, 0x46, 0x76,
	0x55, 0x6b, 0x67, 0x35, 0x54, 0x6b, 0x46, 0x30,
	0x52, 0x48, 0x52, 0x69, 0x57, 0x45, 0x39, 0x59,
	0x4c, 0x31, 0x52, 0x50, 0x56, 0x57, 0x5a, 0x72,
	0x4d, 0x55, 0x67, 0x72, 0x4d, 0x6c, 0x55, 0x78,
	0x51, 0x33, 0x6c, 0x54, 0x62, 0x46, 0x68, 0x57,
	0x55, 0x57, 0x6c, 0x44, 0x4b, 0x30, 0x70, 0x52,
	0x57, 0x47, 0x74, 0x59, 0x4d, 0x55, 0x70, 0x72,
	0x4e, 0x46, 0x64, 0x71, 0x4e, 0x6e, 0x4a, 0x76,
	0x62, 0x31, 0x46, 0x7a, 0x53, 0x33, 0x68, 0x50,
	0x56, 0x57, 0x68, 0x6d, 0x4f, 0x48, 0x52, 0x4f,
	0x5a, 0x47, 0x56, 0x50, 0x59, 0x6d, 0x68, 0x6d,
	0x4d, 0x46, 0x70, 0x47, 0x4d, 0x6c, 0x52, 0x70,
	0x4e, 0x58, 0x42, 0x46, 0x52, 0x6a, 0x52, 0x47,
	0x53, 0x54, 0x5a, 0x55, 0x61, 0x6c, 0x6c, 0x58,
	0x56, 0x31, 0x42, 0x61, 0x59, 0x54, 0x59, 0x72,
	0x64, 0x47, 0x59, 0x33, 0x53, 0x6e, 0x68, 0x54,
	0x4f, 0x56, 0x52, 0x71, 0x59, 0x58, 0x56, 0x6b,
	0x4e, 0x31, 0x4a, 0x4b, 0x4d, 0x45, 0x74, 0x48,
	0x55, 0x6a, 0x5a, 0x72, 0x52, 0x55, 0x46, 0x7a,
	0x63, 0x6d, 0x70, 0x58, 0x62, 0x56, 0x6c, 0x77,
	0x52, 0x55, 0x52, 0x70, 0x63, 0x56, 0x4d, 0x31,
	0x4e, 0x46, 0x52, 0x76, 0x61, 0x6d, 0x6c, 0x6f,
	0x55, 0x30, 0x77, 0x30, 0x5a, 0x55, 0x31, 0x33,
	0x4d, 0x32, 0x49, 0x30, 0x4e, 0x46, 0x55, 0x33,
	0x64, 0x47, 0x6c, 0x4c, 0x57, 0x57, 0x56, 0x48,
	0x59, 0x58, 0x4a, 0x44, 0x4d, 0x6b, 0x52, 0x74,
	0x62, 0x56, 0x64, 0x58, 0x62, 0x6b, 0x74, 0x59,
	0x57, 0x6d, 0x4d, 0x7a, 0x51, 0x57, 0x31, 0x77,
	0x4e, 0x46, 0x70, 0x58, 0x54, 0x48, 0x52, 0x68,
	0x61, 0x53, 0x39, 0x35, 0x5a, 0x7a, 0x52, 0x4e,
	0x62, 0x32, 0x30, 0x32, 0x52, 0x7a, 0x52, 0x5a,
	0x55, 0x43, 0x74, 0x32, 0x4e, 0x46, 0x42, 0x77,
	0x4c, 0x32, 0x4a, 0x49, 0x61, 0x57, 0x64, 0x33,
	0x56, 0x57, 0x70, 0x4e, 0x65, 0x47, 0x64, 0x70,
	0x56, 0x55, 0x5a, 0x75, 0x55, 0x48, 0x4e, 0x49,
	0x64, 0x6d, 0x35, 0x44, 0x62, 0x45, 0x4e, 0x77,
	0x4b, 0x30, 0x5a, 0x53, 0x4f, 0x57, 0x68, 0x6c,
	0x65, 0x46, 0x56, 0x6b, 0x51, 0x6a, 0x49, 0x72,
	0x56, 0x47, 0x77, 0x34, 0x55, 0x48, 0x4a, 0x42,
	0x62, 0x7a, 0x52, 0x72, 0x56, 0x55, 0x6f, 0x33,
	0x4d, 0x45, 0x38, 0x79, 0x62, 0x44, 0x5a, 0x50,
	0x65, 0x47, 0x31, 0x31, 0x4e, 0x46, 0x70, 0x48,
	0x53, 0x6c, 0x59, 0x31, 0x51, 0x57, 0x5a, 0x73,
	0x64, 0x30, 0x35, 0x6c, 0x4d, 0x43, 0x39, 0x73,
	0x4d, 0x31, 0x63, 0x7a, 0x62, 0x30, 0x70, 0x30,
	0x53, 0x45, 0x46, 0x4b, 0x4e, 0x32, 0x46, 0x73,
	0x55, 0x55, 0x6c, 0x45, 0x51, 0x56, 0x46, 0x42,
	0x51, 0x67, 0x41, 0x41, 0x41, 0x51, 0x41, 0x38,
	0x43, 0x41, 0x6d, 0x74, 0x33, 0x65, 0x4e, 0x50,
	0x55, 0x36, 0x50, 0x5a, 0x48, 0x74, 0x71, 0x79,
	0x4e, 0x69, 0x4c, 0x55, 0x44, 0x4e, 0x43, 0x68,
	0x47, 0x41, 0x6a, 0x41, 0x49, 0x5a, 0x67, 0x71,
	0x30, 0x7a, 0x56, 0x74, 0x30, 0x6b, 0x45, 0x72,
	0x68, 0x66, 0x56, 0x45, 0x68, 0x53, 0x6c, 0x5a,
	0x43, 0x7a, 0x4c, 0x52, 0x48, 0x30, 0x67, 0x72,
	0x4f, 0x62, 0x48, 0x43, 0x67, 0x69, 0x53, 0x6f,
	0x53, 0x47, 0x36, 0x35, 0x4d, 0x34, 0x5a, 0x49,
	0x5a, 0x4a, 0x5a, 0x7a, 0x6e, 0x59, 0x5a, 0x34,
	0x6c, 0x53, 0x44, 0x6a, 0x46, 0x7a, 0x4a, 0x32,
	0x58, 0x38, 0x74, 0x53, 0x5a, 0x72, 0x51, 0x4b,
	0x6a, 0x48, 0x31, 0x50, 0x31, 0x79, 0x50, 0x2b,
	0x56, 0x30, 0x44, 0x4c, 0x77, 0x64, 0x73, 0x58,
	0x6d, 0x2b, 0x34, 0x66, 0x6b, 0x6b, 0x38, 0x34,
	0x62, 0x67, 0x72, 0x55, 0x70, 0x52, 0x76, 0x69,
	0x53, 0x38, 0x6f, 0x5a, 0x45, 0x63, 0x63, 0x70,
	0x58, 0x64, 0x72, 0x4e, 0x6e, 0x37, 0x6d, 0x6b,
	0x74, 0x36, 0x68, 0x54, 0x74, 0x34, 0x64, 0x6b,
	0x67, 0x5a, 0x72, 0x6e, 0x6d, 0x31, 0x43, 0x52,
	0x67, 0x55, 0x59, 0x68, 0x55, 0x73, 0x65, 0x7a,
	0x4b, 0x55, 0x72, 0x6d, 0x4e, 0x38, 0x6a, 0x55,
	0x75, 0x43, 0x68, 0x4b, 0x39, 0x2f, 0x65, 0x6f,
	0x71, 0x47, 0x79, 0x31, 0x50, 0x6e, 0x43, 0x4b,
	0x6f, 0x75, 0x4f, 0x74, 0x4f, 0x77, 0x46, 0x5a,
	0x79, 0x68, 0x62, 0x70, 0x2b, 0x55, 0x72, 0x58,
	0x38, 0x6e, 0x49, 0x7a, 0x78, 0x70, 0x2f, 0x4e,
	0x76, 0x75, 0x75, 0x4f, 0x41, 0x43, 0x69, 0x64,
	0x7a, 0x43, 0x6b, 0x56, 0x4e, 0x4b, 0x46, 0x79,
	0x4a, 0x46, 0x50, 0x37, 0x31, 0x70, 0x6f, 0x73,
	0x51, 0x56, 0x30, 0x72, 0x41, 0x50, 0x49, 0x69,
	0x67, 0x67, 0x58, 0x42, 0x30, 0x56, 0x39, 0x4b,
	0x58, 0x45, 0x66, 0x4f, 0x34, 0x74, 0x62, 0x7a,
	0x75, 0x7a, 0x4e, 0x67, 0x4b, 0x63, 0x50, 0x35,
	0x73, 0x37, 0x30, 0x79, 0x43, 0x7a, 0x77, 0x35,
	0x6d, 0x30, 0x2b, 0x6d, 0x58, 0x55, 0x59, 0x75,
	0x72, 0x61, 0x73, 0x79, 0x58, 0x56, 0x54, 0x52,
	0x4b, 0x47, 0x73, 0x6c, 0x33, 0x52, 0x6d, 0x55,
	0x4c, 0x43, 0x6d, 0x6d, 0x4a, 0x6f, 0x53, 0x73,
	0x32, 0x49, 0x32, 0x55, 0x64, 0x2b, 0x35, 0x71,
	0x67, 0x52, 0x4c, 0x47, 0x74, 0x64, 0x4b, 0x2f,
	0x4c, 0x6f, 0x55, 0x4d, 0x69, 0x64, 0x67, 0x6c,
	0x49, 0x79, 0x58, 0x61,
}
