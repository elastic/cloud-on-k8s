// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package fixtures

import (
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/generator"
	"github.com/sethvargo/go-password/password"
)

func DefaultByteGeneratorParams() generator.ByteGeneratorParams {
	return generator.ByteGeneratorParams{
		LowerLetters: password.LowerLetters,
		UpperLetters: password.UpperLetters,
		Symbols:      password.Symbols,
		Digits:       password.Digits,
		Length:       24,
	}
}
