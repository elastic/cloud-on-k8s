// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package password

import (
	"github.com/sethvargo/go-password/password"
)

// DefaultPasswordGeneratorParams returns default parameters for password generation
// * This is to be used for testing purposes only *
func DefaultPasswordGeneratorParams() GeneratorParams {
	return GeneratorParams{
		LowerLetters: password.LowerLetters,
		UpperLetters: password.UpperLetters,
		Symbols:      password.Symbols,
		Digits:       password.Digits,
		Length:       24,
	}
}
