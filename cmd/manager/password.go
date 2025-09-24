// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package manager

import (
	"context"

	"github.com/sethvargo/go-password/password"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	license "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	commonpassword "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password"
)

// getPasswordGenerator returns a password generator based on both the operator flags
// and the license status.
func getPasswordGenerator(ctx context.Context, mgr manager.Manager, operatorNamespace string) (*commonpassword.RandomPasswordGenerator, error) {
	generatorParams, err := validatePasswordFlags(operator.PasswordAllowedCharactersFlag, operator.PasswordLengthFlag)
	if err != nil {
		return nil, err
	}

	enabled, err := license.NewLicenseChecker(mgr.GetClient(), operatorNamespace).EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		enabled = false
	}
	// By initially setting the generator input to nil, the default settings will be used
	// when the enterprise features are not enabled.
	var generatorInput *password.GeneratorInput
	if enabled {
		generatorInput = &password.GeneratorInput{
			LowerLetters: generatorParams.LowerLetters,
			UpperLetters: generatorParams.UpperLetters,
			Digits:       generatorParams.Digits,
			Symbols:      generatorParams.Symbols,
		}
	}
	generator, err := password.NewGenerator(generatorInput)
	if err != nil {
		return nil, err
	}
	return commonpassword.NewRandomPasswordGenerator(generator, generatorParams, enabled), nil
}
