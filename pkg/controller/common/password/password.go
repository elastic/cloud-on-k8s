// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package password

import (
	"context"

	"github.com/sethvargo/go-password/password"
)

// RandomGenerator is an interface for generating random passwords.
type RandomGenerator interface {
	Generate(ctx context.Context) ([]byte, error)
}

// RandomPasswordGenerator is an implementation of RandomGenerator
// that generates random passwords according to the specified parameters
// and according to the Enterprise license level.
type randomPasswordGenerator struct {
	generator password.PasswordGenerator
	useParams func(ctx context.Context) (bool, error)
	params    GeneratorParams
}

var _ RandomGenerator = (*randomPasswordGenerator)(nil)

// Generate returns random password bytes according to the specified parameters
// and according to the Enterprise license level.
func (r *randomPasswordGenerator) Generate(ctx context.Context) ([]byte, error) {
	useParams, err := r.useParams(ctx)
	if err != nil {
		return nil, err
	}
	if !useParams {
		return randomBytes(24), nil
	}

	data, err := r.generator.Generate(
		r.params.Length,
		min(r.params.Length/4, len(r.params.Digits)),  // number of digits to include in the result
		min(r.params.Length/4, len(r.params.Symbols)), // number of symbols to include in the result
		false, // noUpper
		true,  // allowRepeat
	)
	return []byte(data), err
}

// NewRandomPasswordGenerator creates a new instance of RandomPasswordGenerator.
// generator: The password generator to use.
// params: The parameters to use for generating passwords.
// useParams: A function that determines whether to use the parameters or default to non-enterprise functionality.
func NewRandomPasswordGenerator(params GeneratorParams, useParams func(context.Context) (bool, error)) (RandomGenerator, error) {
	generatorInput := &password.GeneratorInput{
		LowerLetters: params.LowerLetters,
		UpperLetters: params.UpperLetters,
		Digits:       params.Digits,
		Symbols:      params.Symbols,
	}
	generator, err := password.NewGenerator(generatorInput)
	if err != nil {
		return nil, err
	}
	return &randomPasswordGenerator{
		generator: generator,
		useParams: useParams,
		params:    params,
	}, nil
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
