// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package password

import (
	"context"

	"github.com/sethvargo/go-password/password"
)

type testGenerator struct {
	length int
	RandomGenerator
	RandomGeneratorWithSetter
}

type RandomGeneratorWithSetter interface {
	RandomGenerator
	SetLength(length int) RandomGenerator
}

func (t *testGenerator) Generate(ctx context.Context) ([]byte, error) {
	data, err := password.Generate(t.length, 10, 0, false, false)
	return []byte(data), err
}

func (t *testGenerator) SetLength(length int) RandomGenerator {
	t.length = length
	return t
}

func TestRandomGenerator() RandomGeneratorWithSetter {
	return &testGenerator{length: 24}
}

// DefaultPasswordGeneratorParams returns default parameters for password generation
// * This is to be used for testing purposes only *
// func DefaultPasswordGeneratorParams() GeneratorParams {
// 	return GeneratorParams{
// 		LowerLetters: password.LowerLetters,
// 		UpperLetters: password.UpperLetters,
// 		Symbols:      password.Symbols,
// 		Digits:       password.Digits,
// 		Length:       24,
// 	}
// }
