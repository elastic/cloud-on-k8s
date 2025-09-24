// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package manager

import (
	"github.com/sethvargo/go-password/password"
	"github.com/spf13/viper"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	license "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	commonpassword "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password"
)

// newPasswordGenerator returns a password generator based on both the operator flags
// and the license status.
func newPasswordGenerator(mgr manager.Manager, operatorNamespace string) (commonpassword.RandomGenerator, error) {
	allowedCharacters := viper.GetString(operator.PasswordAllowedCharactersFlag)
	passwordLength := viper.GetInt(operator.PasswordLengthFlag)
	generatorParams, err := commonpassword.ValidatePasswordFlags(allowedCharacters, passwordLength)
	if err != nil {
		return nil, err
	}

	licenseChecker := license.NewLicenseChecker(mgr.GetClient(), operatorNamespace)
	generatorInput := &password.GeneratorInput{
		LowerLetters: generatorParams.LowerLetters,
		UpperLetters: generatorParams.UpperLetters,
		Digits:       generatorParams.Digits,
		Symbols:      generatorParams.Symbols,
	}
	generator, err := password.NewGenerator(generatorInput)
	if err != nil {
		return nil, err
	}
	return commonpassword.NewRandomPasswordGenerator(generator, generatorParams, licenseChecker.EnterpriseFeaturesEnabled), nil
}
