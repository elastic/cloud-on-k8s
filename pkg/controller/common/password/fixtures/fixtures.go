// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package fixtures

import (
	"context"

	commonpassword "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password"
)

type testGenerator struct {
	length int
	RandomGeneratorWithSetter
}

type RandomGeneratorWithSetter interface {
	commonpassword.RandomGenerator
	SetLength(length int) commonpassword.RandomGenerator
}

func (t *testGenerator) Generate(ctx context.Context) ([]byte, error) {
	generator := commonpassword.MustNewRandomPasswordGenerator(
		commonpassword.GeneratorParams{
			LowerLetters: commonpassword.LowerLetters,
			UpperLetters: commonpassword.UpperLetters,
			Digits:       commonpassword.Digits,
			Length:       t.length,
		}, func(ctx context.Context) (bool, error) { return true, nil })
	return generator.Generate(ctx)
}

func (t *testGenerator) SetLength(length int) commonpassword.RandomGenerator {
	t.length = length
	return t
}

func TestRandomGenerator() RandomGeneratorWithSetter {
	return &testGenerator{length: 24}
}
