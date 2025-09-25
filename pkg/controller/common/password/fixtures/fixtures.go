// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package fixtures

import (
	"context"

	pwgenerator "github.com/m1/go-generate-password/generator"

	commonpassword "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password"
)

type testGenerator struct {
	length int
	commonpassword.RandomGenerator
	RandomGeneratorWithSetter
}

type RandomGeneratorWithSetter interface {
	commonpassword.RandomGenerator
	SetLength(length int) commonpassword.RandomGenerator
}

func (t *testGenerator) Generate(ctx context.Context) ([]byte, error) {
	generator, err := pwgenerator.NewWithDefault()
	if err != nil {
		return nil, err
	}
	data, err := generator.Generate()
	return []byte(*data), err
}

func (t *testGenerator) SetLength(length int) commonpassword.RandomGenerator {
	t.length = length
	return t
}

func TestRandomGenerator() RandomGeneratorWithSetter {
	return &testGenerator{length: 24}
}
