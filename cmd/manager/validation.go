// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package manager

import (
	"fmt"
	"strings"
	"time"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/sethvargo/go-password/password"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
)

func chooseAndValidateIPFamily(ipFamilyStr string, ipFamilyDefault corev1.IPFamily) (corev1.IPFamily, error) {
	switch strings.ToLower(ipFamilyStr) {
	case "":
		return ipFamilyDefault, nil
	case "ipv4":
		return corev1.IPv4Protocol, nil
	case "ipv6":
		return corev1.IPv6Protocol, nil
	default:
		return ipFamilyDefault, fmt.Errorf("IP family can be one of: IPv4, IPv6 or \"\" to auto-detect, but was %s", ipFamilyStr)
	}
}

func validatePasswordFlags(passwordAllowedCharactersFlag string, passwordLengthFlag string) (operator.PasswordGeneratorParams, error) {
	allowedCharacters := viper.GetString(passwordAllowedCharactersFlag)
	generatorParams, other := categorizeAllowedCharacters(allowedCharacters)
	if len(other) > 0 {
		return operator.PasswordGeneratorParams{}, fmt.Errorf("invalid characters in passwords allowed characters: %s", string(other))
	}

	generatorParams.Length = viper.GetInt(passwordLengthFlag)
	// Elasticsearch requires at least 6 characters for passwords
	// https://www.elastic.co/guide/en/elasticsearch/reference/7.5/security-api-put-user.html
	if generatorParams.Length < 6 || generatorParams.Length > 72 {
		return operator.PasswordGeneratorParams{}, fmt.Errorf("password length must be at least 6 and at most 72")
	}

	if len(generatorParams.LowerLetters)+len(generatorParams.UpperLetters)+len(generatorParams.Digits)+len(generatorParams.Symbols) < 10 {
		return operator.PasswordGeneratorParams{}, fmt.Errorf("allowedCharacters for password generation needs to be at least 10 for randomness")
	}

	return generatorParams, nil
}

// categorizeAllowedCharacters categorizes the allowed characters into different categories which
// are needed to use the go-password package properly. It also buckets the 'other' characters into a separate slice
// such that invalid characters are able to be filtered out.
func categorizeAllowedCharacters(s string) (params operator.PasswordGeneratorParams, other []rune) {
	var lowercase, uppercase, digits, symbols []rune

	for _, r := range s {
		switch {
		case strings.ContainsRune(password.LowerLetters, r):
			lowercase = append(lowercase, r)
		case strings.ContainsRune(password.UpperLetters, r):
			uppercase = append(uppercase, r)
		case strings.ContainsRune(password.Digits, r):
			digits = append(digits, r)
		case strings.ContainsRune(password.Symbols, r):
			symbols = append(symbols, r)
		default:
			other = append(other, r)
		}
	}

	return operator.PasswordGeneratorParams{
		LowerLetters: string(lowercase),
		UpperLetters: string(uppercase),
		Digits:       string(digits),
		Symbols:      string(symbols),
	}, other
}

func validateCertExpirationFlags(validityFlag string, rotateBeforeFlag string) (time.Duration, time.Duration, error) {
	certValidity := viper.GetDuration(validityFlag)
	certRotateBefore := viper.GetDuration(rotateBeforeFlag)

	if certRotateBefore > certValidity {
		return certValidity, certRotateBefore, fmt.Errorf("%s must be larger than %s", validityFlag, rotateBeforeFlag)
	}

	return certValidity, certRotateBefore, nil
}
