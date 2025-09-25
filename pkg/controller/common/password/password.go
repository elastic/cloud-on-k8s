// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package password

import (
	"context"
	"fmt"
	"strings"

	pwgenerator "github.com/m1/go-generate-password/generator"
)

const (
	// LowerLetters is the list of lowercase letters.
	LowerLetters = "abcdefghijklmnopqrstuvwxyz"

	// UpperLetters is the list of uppercase letters.
	UpperLetters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	// Digits is the list of permitted digits.
	Digits = "0123456789"

	// Symbols is the list of symbols.
	Symbols = "~!@#$%^&*()_+`-={}|[]\\:\"<>?,./"
)

var (
	defaultConfig = pwgenerator.Config{
		Length:                  uint(24),
		CharacterSet:            strings.Join([]string{LowerLetters, UpperLetters, Digits}, ""),
		IncludeUppercaseLetters: true,
		IncludeLowercaseLetters: true,
		IncludeNumbers:          true,
		IncludeSymbols:          false,
	}
)

// RandomGenerator is an interface for generating random passwords.
type RandomGenerator interface {
	Generate(ctx context.Context) ([]byte, error)
}

// RandomPasswordGenerator is an implementation of RandomGenerator
// that generates random passwords according to the specified parameters
// and according to the Enterprise license level.
type randomPasswordGenerator struct {
	generator *pwgenerator.Generator
	// generator password.PasswordGenerator
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
		return randomBytes(24)
	}

	data, err := r.generator.Generate()
	return []byte(*data), err
}

// NewRandomPasswordGenerator creates a new instance of RandomPasswordGenerator.
// generator: The password generator to use.
// params: The parameters to use for generating passwords.
// useParams: A function that determines whether to use the parameters or default to non-enterprise functionality.
func NewRandomPasswordGenerator(params GeneratorParams, useParams func(context.Context) (bool, error)) (RandomGenerator, error) {
	config := pwgenerator.Config{
		Length:                  uint(params.Length),
		CharacterSet:            strings.Join([]string{params.LowerLetters, params.UpperLetters, params.Digits, params.Symbols}, ""),
		IncludeSymbols:          len(params.Symbols) > 0,
		IncludeUppercaseLetters: len(params.UpperLetters) > 0,
		IncludeLowercaseLetters: len(params.LowerLetters) > 0,
		IncludeNumbers:          len(params.Digits) > 0,
	}
	generator, err := pwgenerator.New(&config)
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
func randomBytes(length int) ([]byte, error) {
	generator, err := pwgenerator.New(&defaultConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create password generator: %w", err)
	}
	data, err := generator.Generate()
	if err != nil {
		return nil, fmt.Errorf("failed to generate password: %w", err)
	}
	return []byte(*data), nil
}
