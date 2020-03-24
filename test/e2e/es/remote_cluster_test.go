// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/stretchr/testify/require"
)

const (
	followerSetting = `{"remote_cluster" : "%s", "leader_index" : "%s"}`
)

// TestRemoteCluster tests the local K8S remote cluster feature.
// 1. In a first Elasticsearch cluster some data are indexed in the "data-integrity-check" index.
// 2. A second cluster is created with the first cluster declared as a remote cluster.
// 3. Finally an index follower is created in the second cluster to follow the "data-integrity-check" index from the first one.
func TestRemoteCluster(t *testing.T) {
	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	name := "test-remote-cluster"
	ns1 := test.Ctx().ManagedNamespace(0)
	es1Builder := elasticsearch.NewBuilder(name).
		WithNamespace(ns1).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithRestrictedSecurityContext()
	es1LicenseTestContext := elasticsearch.NewLicenseTestContext(test.NewK8sClientOrFatal(), es1Builder.Elasticsearch)

	ns2 := test.Ctx().ManagedNamespace(1)
	es2Builder := elasticsearch.NewBuilder(name).
		WithNamespace(ns2).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithRestrictedSecurityContext().
		WithRemoteCluster(es1Builder)
	es2LicenseTestContext := elasticsearch.NewLicenseTestContext(test.NewK8sClientOrFatal(), es2Builder.Elasticsearch)
	licenseBytes, err := ioutil.ReadFile(test.Ctx().TestLicense)
	require.NoError(t, err)
	trialSecretName := "eck-license" // nolint

	before := func(k *test.K8sClient) test.StepList {
		// Deploy a Trial license
		return test.StepList{es1LicenseTestContext.CreateEnterpriseLicenseSecret(trialSecretName, licenseBytes)}
	}

	followerIndex := "data-integrity-check-follower"
	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			// Init license test context
			es1LicenseTestContext.Init(),
			es2LicenseTestContext.Init(),
			// Check that the first cluster is using a Platinum license
			es1LicenseTestContext.CheckElasticsearchLicense(client.ElasticsearchLicenseTypePlatinum),
			// Check that the second cluster is using a Platinum license
			es1LicenseTestContext.CheckElasticsearchLicense(client.ElasticsearchLicenseTypePlatinum),
			test.Step{
				Name: "Add some data to the first cluster",
				Test: func(t *testing.T) {
					// Always enable soft deletes on test index. This is required to create follower indices but disabled by default on 6.x
					require.NoError(t, elasticsearch.NewDataIntegrityCheck(k, es1Builder).WithSoftDeletesEnabled(true).Init())
				},
			},
			test.Step{
				Name: "Create a follower index on the second cluster",
				Test: func(t *testing.T) {
					esClient, err := elasticsearch.NewElasticsearchClient(es2Builder.Elasticsearch, k)
					require.NoError(t, err)
					// create the index with controlled settings
					followerCreation, err := http.NewRequest(
						http.MethodPut,
						fmt.Sprintf("/%s/_ccr/follow", followerIndex),
						bytes.NewBufferString(fmt.Sprintf(followerSetting, es1Builder.Ref().Name, elasticsearch.DataIntegrityIndex)),
					)
					require.NoError(t, err)
					resp, err := esClient.Request(context.Background(), followerCreation)
					require.NoError(t, err)
					defer resp.Body.Close()
				},
			},
			test.Step{
				Name: "Check data in the second cluster",
				Test: test.Eventually(func() error {
					return elasticsearch.NewDataIntegrityCheck(k, es2Builder).ForIndex(followerIndex).Verify()
				}),
			},
			es1LicenseTestContext.DeleteEnterpriseLicenseSecret(trialSecretName),
		}
	}

	test.Sequence(before, stepsFn, es1Builder, es2Builder).RunSequential(t)
}
