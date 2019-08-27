// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

// CheckTestSteps returns all test steps to verify a given resource in K8s is the expected one
// and the given resource is running as expected.
func CheckTestSteps(b Builder, k *K8sClient) StepList {
	return StepList{}.
		WithSteps(b.CheckK8sTestSteps(k)).
		WithSteps(b.CheckStackTestSteps(k))
}
