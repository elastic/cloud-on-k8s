// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
)

func (b Builder) CheckStackSteps(k8sClient *helpers.K8sHelper) helpers.TestStepList {
	return helpers.TestStepList{}.
		WithSteps(K8sStackChecks(b, k8sClient)).
		WithSteps(KibanaChecks(b.Kibana))
}
