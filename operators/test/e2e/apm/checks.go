// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apm

import (
	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
)

func (b Builder) CheckStackSteps(es estype.Elasticsearch, k8sClient *helpers.K8sHelper) helpers.TestStepList {
	return helpers.TestStepList{}.
		WithSteps(K8sStackChecks(b, k8sClient)).
		WithSteps(ApmServerChecks(b.ApmServer, es, k8sClient))
}
