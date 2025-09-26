// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package password

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
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
	defaultCharacterSet = strings.Join([]string{LowerLetters, UpperLetters, Digits}, "")
	DefaultParameters   = GeneratorParams{
		Length:       24,
		LowerLetters: LowerLetters,
		UpperLetters: UpperLetters,
		Digits:       Digits,
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
		return randomBytes()
	}

	return randomBytesWithLength(r.params.Length, strings.Join([]string{r.params.LowerLetters, r.params.UpperLetters, r.params.Digits, r.params.Symbols}, ""))
}

// NewRandomPasswordGenerator creates a new instance of RandomPasswordGenerator.
// params: The parameters to use for generating passwords.
// useParams: A function that determines whether to use the parameters or default to non-enterprise functionality.
func NewRandomPasswordGenerator(params GeneratorParams, useParams func(context.Context) (bool, error)) (RandomGenerator, error) {
	if err := validateParams(params); err != nil {
		return nil, err
	}
	return &randomPasswordGenerator{
		useParams: useParams,
		params:    params,
	}, nil
}

// MustNewRandomPasswordGenerator creates a new instance of RandomPasswordGenerator and panics if it fails.
func MustNewRandomPasswordGenerator(params GeneratorParams, useParams func(context.Context) (bool, error)) RandomGenerator {
	generator, err := NewRandomPasswordGenerator(params, useParams)
	if err != nil {
		panic(err)
	}
	return generator
}

func validateParams(params GeneratorParams) error {
	if params.Length < 6 || params.Length > 72 {
		return fmt.Errorf("password length must be at least 6 and at most 72")
	}
	return nil
}

// randomBytes generates some random bytes that can be used as a token or as a key
// using the default character set which includes lowercase letters, uppercase letters and digits
// but no symbols and a length of 24.
func randomBytes() ([]byte, error) {
	return randomBytesWithLength(24, defaultCharacterSet)
}

// randomBytesWithLength generates some random bytes that can be used as a token or as a key
// using the specified character set and length.
func randomBytesWithLength(length int, characterSet string) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, length))
	for range length {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(length)))
		if err != nil {
			return nil, fmt.Errorf("while generating random data: %w", err)
		}
		buf.WriteByte(characterSet[n.Int64()])
	}
	return buf.Bytes(), nil
}
