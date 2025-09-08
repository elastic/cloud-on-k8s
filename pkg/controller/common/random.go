// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"github.com/sethvargo/go-password/password"
)

type PasswordGeneratorParams struct {
	LowerLetters string
	UpperLetters string
	Digits       string
	Symbols      string
	Length       int
}

// FixedLengthRandomPasswordBytes generates a random password
func FixedLengthRandomPasswordBytes(params PasswordGeneratorParams) []byte {
	return RandomBytes(params)
}

// RandomBytes generates some random bytes that can be used as a token or as a key
func RandomBytes(params PasswordGeneratorParams) []byte {
	generator, err := password.NewGenerator(&password.GeneratorInput{
		LowerLetters: params.LowerLetters,
		UpperLetters: params.UpperLetters,
		Digits:       params.Digits,
		Symbols:      params.Symbols,
	})
	// This is bad. We're going to have to change this func.
	if err != nil {
		panic(err)
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
	))
}
