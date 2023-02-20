// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build fake || e2e

package fake

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func TestFailingStuff(t *testing.T) {
	var steps test.StepList
	steps = []test.Step{
		{
			Name: "failing test",
			Test: func(t *testing.T) {
				t.Error("fake error")
			},
		},
	}
	steps.RunSequential(t)
}

func TestSucceedingStuff(t *testing.T) {
	var steps test.StepList
	steps = []test.Step{
		{
			Name: "failing test",
			Test: func(t *testing.T) {
				t.Log("success")
			},
		},
	}
	steps.RunSequential(t)
}
