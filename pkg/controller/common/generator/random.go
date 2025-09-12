// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package generator

import (
	"context"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/sethvargo/go-password/password"
)

// ByteGeneratorParams defines the parameters for generating random strings
// for uses such as service accounts and passwords.
type ByteGeneratorParams struct {
	LowerLetters string
	UpperLetters string
	Digits       string
	Symbols      string
	Length       int
}

func RandomBytesRespectingLicense(ctx context.Context, client k8s.Client, namespace string, params ByteGeneratorParams) ([]byte, error) {
	enabled, err := license.NewLicenseChecker(client, namespace).EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		return nil, err
	}
	if enabled {
		return RandomBytes(params)
	}
	return BasicLicenseFixedLengthRandomPasswordbytes(24), nil
}

// BasicLicenseFixedLengthRandomPasswordbytes generates a random password with a fixed length of 24 characters
// than is used by users with a basic license.
func BasicLicenseFixedLengthRandomPasswordbytes(length int) []byte {
	return []byte(password.MustGenerate(
		length,
		10,    // number of digits to include in the result
		0,     // number of symbols to include in the result
		false, // noUpper
		true,  // allowRepeat
	))
}

// FixedLengthRandomPasswordBytes generates a random password
func FixedLengthRandomPasswordBytes(params ByteGeneratorParams) []byte {
	return MustRandomBytes(params)
}

// RandomBytes generates some random bytes that can be used as a token or as a key
func MustRandomBytes(params ByteGeneratorParams) []byte {
	results, err := RandomBytes(params)
	if err != nil {
		panic(err)
	}
	return results
}

func RandomBytes(params ByteGeneratorParams) ([]byte, error) {
	generator, err := password.NewGenerator(&password.GeneratorInput{
		LowerLetters: params.LowerLetters,
		UpperLetters: params.UpperLetters,
		Digits:       params.Digits,
		Symbols:      params.Symbols,
	})
	if err != nil {
		return nil, err
	}
	// This indicates to the user that we can have upper and symbols
	// but then we completely disable that here. Why was this setup this
	// way initially?
	return []byte(generator.MustGenerate(
		params.Length,
		10,    // number of digits to include in the result
		0,     // number of symbols to include in the result
		false, // noUpper
		true,  // allowRepeat
	)), nil
}

func RandomPassword(params ByteGeneratorParams) string {
	return string(MustRandomBytes(params))
}
