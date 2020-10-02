// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package esconfig

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	escv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/esconfig/v1alpha1"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

// this is where we actually add things to test
// TODO add clients
func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	var entClient EnterpriseSearchClient
	var appSearchClient AppSearchClient
	return test.StepList{
		test.Step{
			Name: "Every secret should be set so that we can build an Enterprise Search client",
			Test: test.Eventually(func() error {
				var err error
				entClient, err = NewEnterpriseSearchClient(b.EnterpriseSearch, k)
				return err
			}),
		},
