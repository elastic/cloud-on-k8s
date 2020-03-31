// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"io/ioutil"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
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
	licenseSecretName := "eck-e2e-test-license"         // nolint
	updatedLicenseSecretName := "eck-e2e-test-license2" //nolint

	licenseLevelWatch := test.NewWatcher(
		"watch license level never drops to basic",
		1*time.Second,
		func(k *test.K8sClient, t *testing.T) {
			require.NoError(t, licenseTestContext.CheckElasticsearchLicenseFn(
				client.ElasticsearchLicenseTypeGold,
				client.ElasticsearchLicenseTypePlatinum,
			))
		},
		test.NOOPCheck,
	)
	test.StepList{}.
		WithSteps(esBuilder.InitTestSteps(k)).
		// make sure no left over license is still around
		WithStep(licenseTestContext.DeleteAllEnterpriseLicenseSecrets()).
		WithSteps(esBuilder.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(esBuilder, k)).
		WithStep(licenseTestContext.Init()).
		WithSteps(test.StepList{
			licenseTestContext.CheckElasticsearchLicense(client.ElasticsearchLicenseTypeBasic),
			licenseTestContext.CreateEnterpriseLicenseSecret(licenseSecretName, licenseBytes),
			// enterprise license can contain all kinds of cluster licenses so we are a bit lenient here and expect either gold or platinum
			licenseTestContext.CheckElasticsearchLicense(
				client.ElasticsearchLicenseTypeGold,
				client.ElasticsearchLicenseTypePlatinum,
			),
			// but we don't expect to go below that level until we delete the last license secret
			licenseLevelWatch.StartStep(k),
			// simulate an update by creating a second license secret
			licenseTestContext.CreateEnterpriseLicenseSecret(updatedLicenseSecretName, licenseBytes),
			// license level should stay on platinum/gold/enterprise even if we now remove the original secret
			licenseTestContext.DeleteEnterpriseLicenseSecret(licenseSecretName),
			licenseLevelWatch.StopStep(k),
			// and now revert back to basic
			licenseTestContext.DeleteEnterpriseLicenseSecret(updatedLicenseSecretName),
			licenseTestContext.CheckElasticsearchLicense(client.ElasticsearchLicenseTypeBasic),
		}).
		WithSteps(esBuilder.DeletionTestSteps(k)).
		RunSequential(t)
}

// TestEnterpriseTrialLicense this test can be run exactly once per installation!
func TestEnterpriseTrialLicense(t *testing.T) {
	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	licenseBytes, err := ioutil.ReadFile(test.Ctx().TestLicense)
	require.NoError(t, err)

	esBuilder := elasticsearch.NewBuilder("test-es-trial-license").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	var licenseTestContext elasticsearch.LicenseTestContext

	trialSecretName := "eck-trial"
	licenseSecretName := "eck-license"
	initStepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Create license test context",
				Test: func(t *testing.T) {
					licenseTestContext = elasticsearch.NewLicenseTestContext(k, esBuilder.Elasticsearch)
				},
			},
			licenseTestContext.DeleteAllEnterpriseLicenseSecrets(),
			licenseTestContext.CreateEnterpriseTrialLicenseSecret(trialSecretName),
		}
	}

	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			licenseTestContext.Init(),
			licenseTestContext.CheckElasticsearchLicense(client.ElasticsearchLicenseTypeTrial),
			licenseTestContext.CheckEnterpriseTrialLicenseValid(trialSecretName),
			// upgrade from trial to a paid-for license
			licenseTestContext.CreateEnterpriseLicenseSecret(licenseSecretName, licenseBytes),
			licenseTestContext.CheckElasticsearchLicense(
				client.ElasticsearchLicenseTypeGold,
				client.ElasticsearchLicenseTypePlatinum,
			),
			// revert to basic again
			licenseTestContext.DeleteEnterpriseLicenseSecret(trialSecretName),
			licenseTestContext.DeleteEnterpriseLicenseSecret(licenseSecretName),
			licenseTestContext.CheckElasticsearchLicense(client.ElasticsearchLicenseTypeBasic),
			// repeatedly creating a trial is not allowed
			licenseTestContext.CreateEnterpriseTrialLicenseSecret(trialSecretName),
			licenseTestContext.CheckEnterpriseTrialLicenseInvalid(trialSecretName),
		}
	}

	test.Sequence(initStepsFn, stepsFn, esBuilder).RunSequential(t)
}
