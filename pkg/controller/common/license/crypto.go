// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
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
	return append(data, byte(padLen+1)) //nolint:gosec // G115: padLen ≤ customPadding-1 = 19, so padLen+1 ≤ 20 < 256
}

// pkcs5Pad see https://en.wikipedia.org/wiki/Padding_(cryptography)#PKCS#5_and_PKCS#7
func pkcs5Pad(data []byte) []byte {
	padLen := aes.BlockSize - len(data)%aes.BlockSize
	padding := bytes.Repeat([]byte{byte(padLen)}, padLen)
	return append(data, padding...)
}

// decryptWithAESECB is the inverse of encryptWithAESECB.
func decryptWithAESECB(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) == 0 || len(ciphertext)%block.BlockSize() != 0 {
		return nil, errors.New("ciphertext length is not a multiple of the block size")
	}
	plaintext := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += block.BlockSize() {
		block.Decrypt(plaintext[i:], ciphertext[i:])
	}
	// Remove PKCS5 padding. We only validate the last-byte length indicator rather than
	// checking that all padding bytes match, because callers (checkKeyFingerprint) treat
	// any subsequent parse failure as non-fatal and fall through to RSA verification.
	padLen := int(plaintext[len(plaintext)-1])
	if padLen <= 0 || padLen > block.BlockSize() || padLen > len(plaintext) {
		return nil, errors.New("invalid PKCS5 padding")
	}
	plaintext = plaintext[:len(plaintext)-padLen]
	// remove custom padding: the last byte encodes the number of bytes to strip
	if len(plaintext) == 0 {
		return nil, errors.New("empty plaintext after PKCS5 unpadding")
	}
	customPadLen := int(plaintext[len(plaintext)-1])
	if customPadLen <= 0 || customPadLen > len(plaintext) {
		return nil, errors.New("invalid custom padding")
	}
	return plaintext[:len(plaintext)-customPadLen], nil
}
