// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

// ForcedUpgradeTestSteps creates the initial cluster that is not expected to run, wait for conditions to be met,
// then mutates it to the fixed cluster, that is expected to become healthy.
func ForcedUpgradeTestSteps(k *test.K8sClient, initial Builder, conditions []test.Step, fixed Builder) test.StepList {
	return test.StepList{}.
		// create the initial (failing) cluster
		WithSteps(initial.InitTestSteps(k)).
		WithSteps(initial.CreationTestSteps(k)).
		// wait for conditions to be met
		WithSteps(conditions).
		// apply the fixed Elasticsearch resource
		WithSteps(fixed.UpgradeTestSteps(k)).
		// ensure the cluster eventually becomes healthy
		WithSteps(test.CheckTestSteps(fixed, k)).
		// then remove it
		WithSteps(fixed.DeletionTestSteps(k))
}
