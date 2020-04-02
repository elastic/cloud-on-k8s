// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"github.com/sethvargo/go-password/password"
)

// FixedLengthRandomPasswordBytes generates a random password
func FixedLengthRandomPasswordBytes() []byte {
	return RandomBytes(24)
}

// RandomBytes generates some random bytes that can be used as a token or as a key
func RandomBytes(length int) []byte {
	return []byte(password.MustGenerate(
		length,
		10,    // number of digits to include in the result
		0,     // number of symbols to include in the result
		false, // noUpper
		true,  // allowRepeat
	))
}
