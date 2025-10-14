// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package password

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

const (
	// LowerLetters is the list of lowercase letters.
	LowerLetters = "abcdefghijklmnopqrstuvwxyz"

	// UpperLetters is the list of uppercase letters.
	UpperLetters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	// Digits is the list of permitted digits.
	Digits = "0123456789"
)

var (
	characterSetWithoutSymbols = strings.Join([]string{LowerLetters, UpperLetters, Digits}, "")

	// defaultCharacterSet is the default character set used for generating passwords.
	// It includes lowercase letters, uppercase letters and digits, but excludes symbols.
	// This is to ensure compatibility with configurations which do not offer a way to escape symbols.
	defaultCharacterSet = characterSetWithoutSymbols
)

// RandomGenerator is an interface for generating random passwords.
type RandomGenerator interface {
	Generate(ctx context.Context) ([]byte, error)
}

// RandomPasswordGenerator is an implementation of RandomGenerator
// that generates random passwords with the specified length.
type randomPasswordGenerator struct {
	useLength func(ctx context.Context) (bool, error)
	length    int
}

var _ RandomGenerator = (*randomPasswordGenerator)(nil)

// Generate returns random password bytes. It either uses the specified length
// or a default length of 24 characters depending on the results of the useLength function
// provided, which is intended to be licenseCheck.EnterpriseFeaturesEnabled.
func (r *randomPasswordGenerator) Generate(ctx context.Context) ([]byte, error) {
	useLength, err := r.useLength(ctx)
	if err != nil {
		return nil, err
	}
	if useLength {
		return randomBytesWithLengthAndCharset(r.length, defaultCharacterSet)
	}
	return randomBytesWithLengthAndCharset(24, defaultCharacterSet)
}

// NewGenerator returns a password generator with the specified length.
// All character types (lowercase, uppercase, digits) except symbols are used.
func NewGenerator(
	client k8s.Client,
	passwordLength int,
	operatorNamespace string,
) (RandomGenerator, error) {
	licenseChecker := license.NewLicenseChecker(client, operatorNamespace)
	return NewRandomPasswordGenerator(passwordLength, licenseChecker.EnterpriseFeaturesEnabled)
}

// NewRandomPasswordGenerator creates a new instance of RandomPasswordGenerator.
// useLength: A function that determines whether to use length restrictions.
// length: The length of the password to generate.
func NewRandomPasswordGenerator(length int, useLength func(ctx context.Context) (bool, error)) (RandomGenerator, error) {
	if err := validateLength(length); err != nil {
		return nil, err
	}
	return &randomPasswordGenerator{
		useLength: useLength,
		length:    length,
	}, nil
}

// validateLength validates the length for generating passwords.
func validateLength(length int) error {
	// Elasticsearch requires at least 6 characters for passwords
	// https://www.elastic.co/guide/en/elasticsearch/reference/7.5/security-api-put-user.html
	// 72 characters is the upper limit for the bcrypt algorithm, which is used by ECK.
	if length < 6 || length > 72 {
		return fmt.Errorf("password length must be at least 6 and at most 72")
	}
	return nil
}

// RandomBytesWithoutSymbols generates some random bytes that can be used as a token or as a key
// using the character set without symbols and specified length. This is primarily used for
// generating encryption keys and tokens which are based on UUIDV4, which cannot include symbols.
func RandomBytesWithoutSymbols(length int) ([]byte, error) {
	return randomBytesWithLengthAndCharset(length, characterSetWithoutSymbols)
}

// randomBytesWithLength generates some random bytes that can be used as a token or as a key
// using the default character set and specified length.
// Inspired from https://github.com/sethvargo/go-password/blob/v0.3.1/password/generate.go.
func randomBytesWithLengthAndCharset(length int, charSet string) ([]byte, error) {
	b := make([]byte, length)
	for i := range length {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charSet))))
		if err != nil {
			return nil, fmt.Errorf("while generating random data: %w", err)
		}
		b[i] = charSet[n.Int64()]
	}
	return b, nil
}

// MustGenerate is a convenience function for generating random bytes with a specified length
// using the default character set which includes lowercase letters, uppercase letters and digits.
func MustGenerate(length int) []byte {
	b, err := randomBytesWithLengthAndCharset(length, defaultCharacterSet)
	if err != nil {
		panic(err)
	}
	return b
}
