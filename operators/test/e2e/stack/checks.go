// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
)

// CheckStackSteps returns all test steps to verify the status of the given stack
func CheckStackSteps(stack Builder, k8sClient *helpers.K8sHelper) helpers.TestStepList {
	return helpers.TestStepList{}.
		WithSteps(K8sStackChecks(stack, k8sClient)...).
		WithSteps(ESClusterChecks(stack.Elasticsearch, k8sClient)...).
		WithSteps(KibanaChecks(stack.Kibana)...)
}
