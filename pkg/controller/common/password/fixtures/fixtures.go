// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package fixtures

import (
	"context"

	commonpassword "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password"
)

func MustTestRandomGenerator(length int) commonpassword.RandomGenerator {
	generator, err := commonpassword.NewRandomPasswordGenerator(length, func(ctx context.Context) (bool, error) { return true, nil })
	if err != nil {
		panic(err)
	}
	return generator
}
