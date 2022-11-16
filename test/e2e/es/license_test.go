// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"crypto/rsa"
	"crypto/x509"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
)

func TestEnterpriseLicenseSingle(t *testing.T) {
	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	licenseBytes, err := os.ReadFile(test.Ctx().TestLicense)
	require.NoError(t, err)

	k := test.NewK8sClientOrFatal()

	// create a single node cluster
	esBuilder := elasticsearch.NewBuilder("test-es-license-provisioning").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	licenseTestContext := elasticsearch.NewLicenseTestContext(k, esBuilder.Elasticsearch)
	licenseSecretName := "eck-e2e-test-license"         // nolint
	updatedLicenseSecretName := "eck-e2e-test-license2" // nolint

	licenseLevelWatch := test.NewWatcher(
		"watch license level never drops to basic",
		1*time.Second,
		func(k *test.K8sClient, t *testing.T) {
			require.NoError(t, licenseTestContext.CheckElasticsearchLicenseFn(
				client.ElasticsearchLicenseTypeGold,       // pre 8.0 for backwards compatibility gold/platinum was reported
				client.ElasticsearchLicenseTypePlatinum,   // even if an enterprise license was installed
				client.ElasticsearchLicenseTypeEnterprise, // as of 8.0 enterprise is reported
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
				client.ElasticsearchLicenseTypeEnterprise, // as of 8.0 see comment above
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
	// but do not execute if we have a private key to generate trial extensions (see TestEnterpriseTrialExtension)
	if test.Ctx().TestLicense == "" || test.Ctx().TestLicensePKeyPath != "" {
		t.SkipNow()
	}

	licenseBytes, err := os.ReadFile(test.Ctx().TestLicense)
	require.NoError(t, err)

	esBuilder := elasticsearch.NewBuilder("test-es-trial-license").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	licenseTestContext := elasticsearch.NewLicenseTestContext(test.NewK8sClientOrFatal(), esBuilder.Elasticsearch)

	trialSecretName := "eck-trial"
	licenseSecretName := "eck-license"
	initStepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
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
				client.ElasticsearchLicenseTypeEnterprise, // as of 8.0 see comment above
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

// TestEnterpriseTrialExtension tests that trial extensions can be successfully applied and take effect.
// Starts and verifies an ECK-managed trial. Tests that license is applied and test cluster is running in trial mode.
// Then generates a development version of an Enterprise trial extension license with a development Elasticsearch license inside.
// Then tests that ECK accepts this license and propagates the Elasticsearch license to the test Elasticsearch cluster.
// Finally tests that trial extensions can be applied repeatedly as opposed to ECK-managed trials which are one-offs.
func TestEnterpriseTrialExtension(t *testing.T) {
	if test.Ctx().TestLicensePKeyPath == "" {
		// skip this test if the dev private key is not configured e.g. because we are testing a production build
		t.SkipNow()
	}
	privateKeyBytes, err := os.ReadFile(test.Ctx().TestLicensePKeyPath)
	require.NoError(t, err)
	privateKey, err := x509.ParsePKCS8PrivateKey(privateKeyBytes)
	require.NoError(t, err)

	esBuilder := elasticsearch.NewBuilder("test-es-trial-extension").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	licenseTestContext := elasticsearch.NewLicenseTestContext(test.NewK8sClientOrFatal(), esBuilder.Elasticsearch)

	trialSecretName := "eck-trial"
	licenseSecretName := "eck-license"
	initStepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			licenseTestContext.DeleteAllEnterpriseLicenseSecrets(),
			licenseTestContext.CreateEnterpriseTrialLicenseSecret(trialSecretName),
		}
	}

	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			licenseTestContext.Init(),
			licenseTestContext.CheckElasticsearchLicense(client.ElasticsearchLicenseTypeTrial),
			licenseTestContext.CheckEnterpriseTrialLicenseValid(trialSecretName),
			// simulate a trial extension: old style with Enterprise Elasticsearch license
			licenseTestContext.CreateTrialExtension(licenseSecretName, privateKey.(*rsa.PrivateKey), client.ElasticsearchLicenseTypeEnterprise),
			licenseTestContext.CheckElasticsearchLicense(
				client.ElasticsearchLicenseTypePlatinum, // depends on ES version
				client.ElasticsearchLicenseTypeEnterprise,
			),
			// revert to basic again
			licenseTestContext.DeleteAllEnterpriseLicenseSecrets(),
			licenseTestContext.CheckElasticsearchLicense(client.ElasticsearchLicenseTypeBasic),
			// repeatedly extending a trial is possible: new style with Elasticsearch trial license
			licenseTestContext.CreateTrialExtension(licenseSecretName, privateKey.(*rsa.PrivateKey), client.ElasticsearchLicenseTypeTrial),
			licenseTestContext.CheckElasticsearchLicense(
				client.ElasticsearchLicenseTypeTrial,
			),
			// cleanup license for the next tests
			licenseTestContext.DeleteAllEnterpriseLicenseSecrets(),
		}
	}

	test.Sequence(initStepsFn, stepsFn, esBuilder).RunSequential(t)
}
