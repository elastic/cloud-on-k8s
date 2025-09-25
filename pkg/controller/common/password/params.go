// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package password

import (
	"fmt"
	"strings"
)

// GeneratorParams defines the parameters for generating random passwords.
type GeneratorParams struct {
	LowerLetters string
	UpperLetters string
	Digits       string
	Symbols      string
	Length       int
}

func NewGeneratorParams(allowedCharacter string, maxLength int) (GeneratorParams, error) {
	generatorParams, other := categorizeAllowedCharacters(allowedCharacter)
	if len(other) > 0 {
		return GeneratorParams{}, fmt.Errorf("invalid characters in passwords allowed characters: %s", string(other))
	}
	generatorParams.Length = maxLength
	// Elasticsearch requires at least 6 characters for passwords
	// https://www.elastic.co/guide/en/elasticsearch/reference/7.5/security-api-put-user.html
	if generatorParams.Length < 6 || generatorParams.Length > 72 {
		return GeneratorParams{}, fmt.Errorf("password length must be at least 6 and at most 72")
	}

	if len(generatorParams.LowerLetters)+len(generatorParams.UpperLetters)+len(generatorParams.Digits)+len(generatorParams.Symbols) < 10 {
		return GeneratorParams{}, fmt.Errorf("allowedCharacters for password generation needs to be at least 10 for randomness")
	}
	return generatorParams, nil
}

// categorizeAllowedCharacters categorizes the allowed characters into different categories which
// are needed to use the password generator package properly. It also buckets the 'other' characters into a separate slice
// such that invalid characters are able to be filtered out.
func categorizeAllowedCharacters(s string) (params GeneratorParams, other []rune) {
	var lowercase, uppercase, digits, symbols []rune

	for _, r := range s {
		switch {
		case strings.ContainsRune(LowerLetters, r):
			lowercase = append(lowercase, r)
		case strings.ContainsRune(UpperLetters, r):
			uppercase = append(uppercase, r)
		case strings.ContainsRune(Digits, r):
			digits = append(digits, r)
		case strings.ContainsRune(Symbols, r):
			symbols = append(symbols, r)
		default:
			other = append(other, r)
		}
	}

	return GeneratorParams{
		LowerLetters: string(lowercase),
		UpperLetters: string(uppercase),
		Digits:       string(digits),
		Symbols:      string(symbols),
	}, other
}
