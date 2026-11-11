// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestValidateRejectsSkipOCIExistingWithForce(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Set(envFlag, devEnvironment)
	viper.Set(skipOCIExistingFlag, true)
	viper.Set(forceFlag, true)

	err := validate(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("expected an error about --%s and --%s, got: %v", skipOCIExistingFlag, forceFlag, err)
	}
}
