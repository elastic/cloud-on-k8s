// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import "github.com/elastic/cloud-on-k8s/test/e2e/test"

func (b Builder) MutationTestSteps(k *test.K8sClient) test.StepList {
	panic("not implemented")
}

func (b Builder) UpgradeTestSteps(k *test.K8sClient) test.StepList {
	panic("not implemented")
}

func (b Builder) MutationReversalTestContext() test.ReversalTestContext {
	panic("not implemented")
}
