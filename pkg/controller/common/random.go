// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"github.com/sethvargo/go-password/password"
)

// PasswordGeneratorParams defines the parameters for generating random passwords.
type PasswordGeneratorParams struct {
	LowerLetters string
	UpperLetters string
	Digits       string
	Symbols      string
	Length       int
}

// FixedLengthRandomPasswordBytes generates a random password
func FixedLengthRandomPasswordBytes(params PasswordGeneratorParams) []byte {
	return MustRandomBytes(params)
}

// RandomBytes generates some random bytes that can be used as a token or as a key
func MustRandomBytes(params PasswordGeneratorParams) []byte {
	results, err := RandomBytes(params)
	if err != nil {
		panic(err)
	}
	return results
}

func RandomBytes(params PasswordGeneratorParams) ([]byte, error) {
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

func RandomPassword(params PasswordGeneratorParams) string {
	return string(MustRandomBytes(params))
}
