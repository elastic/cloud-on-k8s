// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"io/ioutil"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
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
	initEs := elasticsearch.NewBuilder("test-es-license-provisioning").
		WithESMasterNodes(1, elasticsearch.DefaultResources)

	mutatedEs := initEs.
		WithNoESTopology().
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	testContext := elasticsearch.NewLicenseTestContext(k, initEs.Elasticsearch)

	helpers.TestStepList{}.
		WithSteps(initEs.InitTestSteps(k)).
		// make sure no left over license is still around
		WithStep(testContext.DeleteEnterpriseLicenseSecret()).
		WithSteps(initEs.CreationTestSteps(k)).
		WithStep(testContext.Init()).
		WithSteps(helpers.TestStepList{
			testContext.CheckElasticsearchLicense(license.ElasticsearchLicenseTypeBasic),
			testContext.CreateEnterpriseLicenseSecret(licenseBytes),
		}).
		// Mutation shortcuts the license provisioning check...
		WithSteps(mutatedEs.MutationTestSteps(k)).
		// enterprise license can contain all kinds of cluster licenses so we are a bit lenient here and expect either gold or platinum
		WithStep(testContext.CheckElasticsearchLicense(
			license.ElasticsearchLicenseTypeGold,
			license.ElasticsearchLicenseTypePlatinum,
		)).
		WithSteps(mutatedEs.DeletionTestSteps(k)).
		RunSequential(t)

}
