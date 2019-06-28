// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"io/ioutil"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework/elasticsearch"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseLicenseSingle(t *testing.T) {
	// only execute this test if we have a test license to work with
	if framework.TestLicense == "" {
		t.SkipNow()
	}
	k := framework.NewK8sClientOrFatal()

	licenseBytes, err := ioutil.ReadFile(framework.TestLicense)
	require.NoError(t, err)

	// create a single node cluster
	esBuilder := elasticsearch.NewBuilder("test-es-license-provisioning").
		WithESMasterNodes(1, elasticsearch.DefaultResources)

	mutatedEsBuilder := esBuilder.
		WithNoESTopology().
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	licenseTestContext := elasticsearch.NewLicenseTestContext(k, esBuilder.Elasticsearch)

	framework.TestStepList{}.
		WithSteps(esBuilder.InitTestSteps(k)).
		// make sure no left over license is still around
		WithStep(licenseTestContext.DeleteEnterpriseLicenseSecret()).
		WithSteps(esBuilder.CreationTestSteps(k)).
		WithSteps(framework.CheckTestSteps(esBuilder, k)).
		WithStep(licenseTestContext.Init()).
		WithSteps(framework.TestStepList{
			licenseTestContext.CheckElasticsearchLicense(license.ElasticsearchLicenseTypeBasic),
			licenseTestContext.CreateEnterpriseLicenseSecret(licenseBytes),
		}).
		// Mutation shortcuts the license provisioning check...
		WithSteps(mutatedEsBuilder.MutationTestSteps(k)).
		// enterprise license can contain all kinds of cluster licenses so we are a bit lenient here and expect either gold or platinum
		WithStep(licenseTestContext.CheckElasticsearchLicense(
			license.ElasticsearchLicenseTypeGold,
			license.ElasticsearchLicenseTypePlatinum,
		)).
		WithSteps(mutatedEsBuilder.DeletionTestSteps(k)).
		RunSequential(t)

}
