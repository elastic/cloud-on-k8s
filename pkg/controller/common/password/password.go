// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package password

import (
	"github.com/sethvargo/go-password/password"
)

type RandomGenerator interface {
	Generate() ([]byte, error)
	SetEnterpriseEnabled(enabled bool)
}

type RandomPasswordGenerator struct {
	generator         password.PasswordGenerator
	enterpriseLicense bool
	params            PasswordGeneratorParams
}

var _ RandomGenerator = (*RandomPasswordGenerator)(nil)

// PasswordGeneratorParams defines the parameters for generating random passwords.
type PasswordGeneratorParams struct {
	LowerLetters string
	UpperLetters string
	Digits       string
	Symbols      string
	Length       int
}

func (r *RandomPasswordGenerator) Generate() ([]byte, error) {
	if r.enterpriseLicense {
		data, err := r.generator.Generate(
			r.params.Length,
			min(r.params.Length/4, len(r.params.Digits)),  // number of digits to include in the result
			min(r.params.Length/4, len(r.params.Symbols)), // number of symbols to include in the result
			false, // noUpper
			true,  // allowRepeat
		)
		return []byte(data), err
	}
	return randomBytes(24), nil
}

func (r *RandomPasswordGenerator) SetEnterpriseEnabled(enabled bool) {
	r.enterpriseLicense = enabled
}

func NewRandomPasswordGenerator(generator password.PasswordGenerator, params PasswordGeneratorParams, enterpriseLicense bool) *RandomPasswordGenerator {
	return &RandomPasswordGenerator{
		generator:         generator,
		enterpriseLicense: enterpriseLicense,
		params:            params,
	}
}

// randomBytes generates some random bytes that can be used as a token or as a key
func randomBytes(length int) []byte {
	return []byte(password.MustGenerate(
		length,
		10,    // number of digits to include in the result
		0,     // number of symbols to include in the result
		false, // noUpper
		true,  // allowRepeat
	))
}
