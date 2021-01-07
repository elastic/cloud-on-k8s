// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
)

const customPadding = 20

var (
	// aesKey is effectively constant
	// see https://github.com/elastic/elasticsearch/blob/a4199af58b8c5ab4757a45248672e0233978b208/x-pack/plugin/core/src/main/java/org/elasticsearch/license/CryptUtils.java#L162-L179
	aesKey = []byte("\x76\x7A\x57\x69\x68\x4A\x4A\x4B\x6E\x30\x72\x6B\x61\x69\x50\x2B")
)

// encryptWithAESECB should be identical to what is called v3 license key
// encryption see https://github.com/elastic/elasticsearch/blob/a4199af58b8c5ab4757a45248672e0233978b208/x-pack/plugin/core/src/main/java/org/elasticsearch/license/CryptUtils.java#L142-L180
func encryptWithAESECB(plaintext []byte) ([]byte, error) {
	padded := pkcs5Pad(customPad(plaintext))
	ciphertext := make([]byte, len(padded))

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, err
	}

	ecb(block, ciphertext, padded)
	return ciphertext, nil
}

func ecb(block cipher.Block, dst, src []byte) {
	// ECB encrypts each block independently https://en.wikipedia.org/wiki/Block_cipher_mode_of_operation#Electronic_codebook_(ECB)
	for len(src) > 0 {
		block.Encrypt(dst, src[:block.BlockSize()])
		src = src[block.BlockSize():]
		dst = dst[block.BlockSize():]
	}
}

// customPad see https://github.com/elastic/elasticsearch/blob/a4199af58b8c5ab4757a45248672e0233978b208/x-pack/plugin/core/src/main/java/org/elasticsearch/license/CryptUtils.java#L212-L236
func customPad(data []byte) []byte {
	if len(data) >= customPadding {
		return append(data, 1)
	}
	padLen := customPadding - len(data)
	padding := make([]byte, padLen)
	_, _ = rand.Read(padding)
	data = append(data, padding...)
	return append(data, byte(padLen+1))

}

// pkcs5Pad see https://en.wikipedia.org/wiki/Padding_(cryptography)#PKCS#5_and_PKCS#7
func pkcs5Pad(data []byte) []byte {
	padLen := aes.BlockSize - len(data)%aes.BlockSize
	padding := bytes.Repeat([]byte{byte(padLen)}, padLen)
	return append(data, padding...)
}
