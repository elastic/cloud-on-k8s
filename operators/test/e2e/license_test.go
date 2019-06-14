/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package e2e

import (
	"io/ioutil"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/stack"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseLicenseSingle(t *testing.T) {
	// only execute this test if we have a test license to work with
	if params.TestLicense == "" {
		t.SkipNow()
	}
	k := helpers.NewK8sClientOrFatal()

	licenseBytes, err := ioutil.ReadFile(params.TestLicense)
	require.NoError(t, err)

	// create a single node cluster
	s := stack.NewStackBuilder("test-es-license-provisioning").
		WithESMasterDataNodes(1, stack.DefaultResources)

	testContext := stack.NewLicenseTestContext(k, s.Elasticsearch)

	helpers.TestStepList{}.
		WithSteps(stack.InitTestSteps(s, k)...).
		WithSteps(stack.CreationTestSteps(s, k)...).
		WithSteps(
			testContext.WrapSteps(
				testContext.CheckElasticsearchLicense(license.ElasticsearchLicenseTypeBasic),
				testContext.CreateEnterpriseLicenseSecret(licenseBytes),
				// enterprise license can contain all kinds of cluster licenses so we are a bit lenient here and expect either gold or platinum
				testContext.CheckElasticsearchLicense(license.ElasticsearchLicenseTypeGold, license.ElasticsearchLicenseTypePlatinum),
			)...).
		RunSequential(t)

}
