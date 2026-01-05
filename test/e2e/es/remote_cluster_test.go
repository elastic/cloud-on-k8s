// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

const (
	followerSetting = `{"remote_cluster" : "%s", "leader_index" : "%s"}`
)

// TestRemoteCluster tests the local K8S remote cluster feature.
// 1. In a first Elasticsearch cluster some data are indexed in the "data-integrity-check" index.
// 2. A second cluster is created with the first cluster declared as a remote cluster.
// 3. Finally, an index follower is created in the second cluster to follow the "data-integrity-check" index from the first one.
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
	licenseBytes, err := os.ReadFile(test.Ctx().TestLicense)
	require.NoError(t, err)
	trialSecretName := "eck-license"

	before := func(k *test.K8sClient) test.StepList {
		// Deploy a Trial license
		return test.StepList{
			es1LicenseTestContext.DeleteAllEnterpriseLicenseSecrets(),
			es1LicenseTestContext.CreateEnterpriseLicenseSecret(trialSecretName, licenseBytes),
		}
	}

	followerIndex := "data-integrity-check-follower"
	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			// Init license test context
			es1LicenseTestContext.Init(),
			es2LicenseTestContext.Init(),
			// Check that the first cluster is using a Platinum license
			es1LicenseTestContext.CheckElasticsearchLicense(
				client.ElasticsearchLicenseTypePlatinum,
				client.ElasticsearchLicenseTypeEnterprise,
			),
			// Check that the second cluster is using a Platinum license
			es1LicenseTestContext.CheckElasticsearchLicense(
				client.ElasticsearchLicenseTypePlatinum,
				client.ElasticsearchLicenseTypeEnterprise,
			),
			test.Step{
				Name: "Add some data to the first cluster",
				Test: func(t *testing.T) {
					// Always enable soft deletes on test index. This is required to create follower indices.
					require.NoError(t, elasticsearch.NewDataIntegrityCheck(k, es1Builder).WithSoftDeletesEnabled(true).Init())
				},
			},
			test.Step{
				Name: "Check that remote cluster is connected using transport protocol",
				Test: test.Eventually(checkRemoteClusterSeed(k, es2Builder, es1Builder, 9300)),
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

// TestRemoteCluster tests the K8S remote cluster feature using API Keys.
// This test is similar to TestRemoteCluster:
// 1. In a first Elasticsearch cluster some data are indexed in the "data-integrity-check" index.
// 2. A second cluster is created with the first cluster declared as a remote cluster.
// 3. Finally, an index follower is created in the second cluster to follow the "data-integrity-check" index from the first one.
func TestRemoteClusterWithAPIKeys(t *testing.T) {
	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	// only execute the test if Elasticsearch supports API keys
	if version.MustParse(test.Ctx().ElasticStackVersion).LT(esv1.RemoteClusterAPIKeysMinVersion) {
		t.Skipf("%s does not support remote cluster API keys", esv1.RemoteClusterAPIKeysMinVersion)
	}

	name := "test-remote-cluster-api-keys"
	ns1 := test.Ctx().ManagedNamespace(0)
	es1Builder := elasticsearch.NewBuilder(name).
		WithNamespace(ns1).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithRemoteClusterServer().
		WithRestrictedSecurityContext()
	es1LicenseTestContext := elasticsearch.NewLicenseTestContext(test.NewK8sClientOrFatal(), es1Builder.Elasticsearch)

	ns2 := test.Ctx().ManagedNamespace(1)
	es2Builder := elasticsearch.NewBuilder(name).
		WithNamespace(ns2).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithRestrictedSecurityContext().
		WithRemoteClusterAPIKey(es1Builder, &esv1.RemoteClusterAPIKey{
			Access: esv1.RemoteClusterAccess{
				Search: &esv1.Search{
					Names: []string{elasticsearch.DataIntegrityIndex},
				},
				Replication: &esv1.Replication{
					Names: []string{elasticsearch.DataIntegrityIndex},
				},
			},
		})
	es2LicenseTestContext := elasticsearch.NewLicenseTestContext(test.NewK8sClientOrFatal(), es2Builder.Elasticsearch)
	licenseBytes, err := os.ReadFile(test.Ctx().TestLicense)
	require.NoError(t, err)
	trialSecretName := "eck-license"

	before := func(k *test.K8sClient) test.StepList {
		// Deploy a Trial license
		return test.StepList{
			es1LicenseTestContext.DeleteAllEnterpriseLicenseSecrets(),
			es1LicenseTestContext.CreateEnterpriseLicenseSecret(trialSecretName, licenseBytes),
		}
	}

	followerIndex := "data-integrity-check-follower"
	remoteIndexName := fmt.Sprintf("%s:%s", es1Builder.Name(), elasticsearch.DataIntegrityIndex)
	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			// Init license test context
			es1LicenseTestContext.Init(),
			es2LicenseTestContext.Init(),
			// Check that the first cluster is using a Platinum license
			es1LicenseTestContext.CheckElasticsearchLicense(
				client.ElasticsearchLicenseTypeEnterprise,
			),
			// Check that the second cluster is using a Platinum license
			es1LicenseTestContext.CheckElasticsearchLicense(
				client.ElasticsearchLicenseTypeEnterprise,
			),
			test.Step{
				Name: "Add some data to the first cluster",
				Test: func(t *testing.T) {
					require.NoError(t, elasticsearch.NewDataIntegrityCheck(k, es1Builder).Init())
				},
			},
			test.Step{
				Name: "Check that remote cluster is connected to remote cluster server",
				Test: test.Eventually(checkRemoteClusterSeed(k, es2Builder, es1Builder, 9443)),
			},
			test.Step{
				Name: fmt.Sprintf("Check we can read remote index %s from the client cluster", remoteIndexName),
				Test: test.Eventually(func() error {
					return elasticsearch.NewDataIntegrityCheck(k, es2Builder).ForIndex(remoteIndexName).Verify()
				}),
			},
			test.Step{
				Name: "Create a follower index in the client cluster",
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
				Name: "Check data in the following index in the client cluster",
				Test: test.Eventually(func() error {
					return elasticsearch.NewDataIntegrityCheck(k, es2Builder).ForIndex(followerIndex).Verify()
				}),
			},
			es1LicenseTestContext.DeleteEnterpriseLicenseSecret(trialSecretName),
		}
	}

	test.Sequence(before, stepsFn, es1Builder, es2Builder).RunSequential(t)
}

func checkRemoteClusterSeed(k *test.K8sClient, clientES, remoteES elasticsearch.Builder, expectedPort int) func() error {
	return func() error {
		esClient, err := elasticsearch.NewElasticsearchClient(clientES.Elasticsearch, k)
		if err != nil {
			return err
		}
		settings, err := esClient.GetRemoteClusterSettings(context.Background())
		if err != nil {
			return err
		}
		persistentSettings := settings.PersistentSettings
		if persistentSettings == nil {
			return fmt.Errorf("no persistent settings found in client cluster %s/%s", clientES.Elasticsearch.Namespace, clientES.Elasticsearch.Name)
		}
		cluster, ok := persistentSettings.Cluster.RemoteClusters[remoteES.Name()]
		if !ok {
			return fmt.Errorf("client cluster %s not found in persistent settings", remoteES.Name())
		}
		if len(cluster.Seeds) == 0 {
			return fmt.Errorf("no seed for client cluster %s found in persistent settings", remoteES.Name())
		}
		expectedSuffix := fmt.Sprintf(":%d", expectedPort)
		for _, seed := range cluster.Seeds {
			if !strings.HasSuffix(seed, expectedSuffix) {
				return fmt.Errorf("client cluster seed must end with %s, found \"%s\"", expectedSuffix, seed)
			}
		}
		return nil
	}
}
