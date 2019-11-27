// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"io/ioutil"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseLicenseSingle(t *testing.T) {
	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}
	k := test.NewK8sClientOrFatal()

	licenseBytes, err := ioutil.ReadFile(test.Ctx().TestLicense)
	require.NoError(t, err)

	// create a single node cluster
	esBuilder := elasticsearch.NewBuilder("test-es-license-provisioning").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	licenseTestContext := elasticsearch.NewLicenseTestContext(k, esBuilder.Elasticsearch)

	test.StepList{}.
		WithSteps(esBuilder.InitTestSteps(k)).
		// make sure no left over license is still around
		WithStep(licenseTestContext.DeleteEnterpriseLicenseSecret()).
		WithSteps(esBuilder.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(esBuilder, k)).
		WithStep(licenseTestContext.Init()).
		WithSteps(test.StepList{
			licenseTestContext.CheckElasticsearchLicense(license.ElasticsearchLicenseTypeBasic),
			licenseTestContext.CreateEnterpriseLicenseSecret(licenseBytes),
			// enterprise license can contain all kinds of cluster licenses so we are a bit lenient here and expect either gold or platinum
			licenseTestContext.CheckElasticsearchLicense(
				license.ElasticsearchLicenseTypeGold,
				license.ElasticsearchLicenseTypePlatinum,
			),
		}).
		WithSteps(esBuilder.DeletionTestSteps(k)).
		WithStep(licenseTestContext.DeleteEnterpriseLicenseSecret()).
		RunSequential(t)
}

func TestEnterpriseTrialLicense(t *testing.T) {
	esBuilder := elasticsearch.NewBuilder("test-es-trial-license").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	var licenseTestContext elasticsearch.LicenseTestContext

	initStepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Create license test context",
				Test: func(t *testing.T) {
					licenseTestContext = elasticsearch.NewLicenseTestContext(k, esBuilder.Elasticsearch)
				},
			},
			licenseTestContext.DeleteEnterpriseLicenseSecret(),
			licenseTestContext.CreateEnterpriseTrialLicenseSecret(),
		}
	}

	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			licenseTestContext.Init(),
			licenseTestContext.CheckElasticsearchLicense(license.ElasticsearchLicenseTypeTrial),
		}
	}

	test.Sequence(initStepsFn, stepsFn, esBuilder).RunSequential(t)
}
