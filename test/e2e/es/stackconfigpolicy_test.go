// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	esClient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
)

// TestStackConfigPolicy ...
func TestStackConfigPolicy(t *testing.T) {
	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		fmt.Println("no license")
		t.SkipNow()
	}

	// StackConfigPolicy is supported since 8.6.0
	stackVersion := version.MustParse(test.Ctx().ElasticStackVersion)
	if !stackVersion.GTE(version.MustParse("8.6.0-SNAPSHOT")) {
		fmt.Println("invalid version")
		t.SkipNow()
	}

	es := elasticsearch.NewBuilder("test-es-scp").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithLabel("label", "test-scp")

	policyNamespace := test.Ctx().ManagedNamespace(0)
	policyName := fmt.Sprintf("test-scp-%s", rand.String(4))
	repoName := "repo-test"
	expectedMaxBytesPerSec := "42mb"

	policy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: policyNamespace,
			Name:      policyName,
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			ResourceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"label": "test-scp"},
			},
			SecureSettings: []commonv1.SecretSource{
				{SecretName: "secure-settings-1"},
			},
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				ClusterSettings: &commonv1.Config{Data: map[string]interface{}{
					"indices.recovery.max_bytes_per_sec": expectedMaxBytesPerSec,
				}},
				SnapshotRepositories: &commonv1.Config{Data: map[string]interface{}{
					repoName: map[string]interface{}{
						"type": "gcs",
						"settings": map[string]interface{}{
							"bucket": "bucket-test",
						},
					}},
				},
			},
		},
	}

	secureSettingsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("test-scp-secure-settings-%s", rand.String(4)),
			Namespace: test.Ctx().ManagedNamespace(1),
		},
		Data: map[string][]byte{
			"gcs.client.secondary.credentials_file": []byte(`{
				"type": "service_account",
				"project_id": "PROJECT_ID",
				"private_key_id": "KEY_ID",
				"private_key": "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC2y8v2zfDTwdIa\n-----END PRIVATE KEY-----\n",
				"client_email": "SERVICE_ACCOUNT_EMAIL",
				"client_id": "CLIENT_ID",
				"auth_uri": "https://accounts.google.com/o/oauth2/auth",
				"token_uri": "https://accounts.google.com/o/oauth2/token",
				"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
				"client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/SERVICE_ACCOUNT_EMAIL"
        	}`),
		},
	}

	expectedRepo := SnapshotRepository{
		Type: "gcs",
		Settings: SnapshotRepositorySettings{
			Bucket:   "bucket-test",
			BasePath: fmt.Sprintf("snapshots/%s-%s", policyNamespace, policyName)},
	}
	steps := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			test.Step{
				Name: "Create a Secure Settings secret",
				Test: test.Eventually(func() error {
					err := k.CreateOrUpdate(&secureSettingsSecret)
					return err
				}),
			},
			test.Step{
				Name: "Create a StackConfigPolicy",
				Test: test.Eventually(func() error {
					err := k.CreateOrUpdate(&policy)
					return err
				}),
			},
			test.Step{
				Name: "Cluster settings should be configured",
				Test: test.Eventually(func() error {
					esClient, err := elasticsearch.NewElasticsearchClient(es.Elasticsearch, k)
					assert.NoError(t, err)

					var settings ClusterSettings
					err = Request(esClient, http.MethodGet, "/_cluster/settings", nil, &settings)
					if err != nil {
						return err
					}

					if settings.Persistent.Indices.Recovery.MaxBytesPerSec != expectedMaxBytesPerSec {
						return errors.New("cluster settings not configured")
					}
					return nil
				}),
			},
			test.Step{
				Name: "Snapshot repository should be configured",
				Test: test.Eventually(func() error {
					esClient, err := elasticsearch.NewElasticsearchClient(es.Elasticsearch, k)
					assert.NoError(t, err)

					var repos SnapshotRepositories
					err = Request(esClient, http.MethodGet, filepath.Join("/_snapshot", repoName), nil, &repos)
					if err != nil {
						return err
					}

					if !reflect.DeepEqual(repos[repoName], expectedRepo) {
						return errors.New("snapshot repository not configured")
					}
					return nil
				}),
			},
			test.Step{
				Name: "Deleting the StackConfigPolicy should return no error",
				Test: test.Eventually(func() error {
					return k.Client.Delete(context.Background(), &policy)
				}),
			},
			test.Step{
				Name: "Cluster settings should be reset",
				Test: test.Eventually(func() error {
					esClient, err := elasticsearch.NewElasticsearchClient(es.Elasticsearch, k)
					assert.NoError(t, err)

					var settings ClusterSettings
					err = Request(esClient, http.MethodGet, "/_cluster/settings", nil, &settings)
					if err != nil {
						return err
					}

					if !reflect.DeepEqual(settings, ClusterSettings{}) {
						return errors.New("cluster settings not reset")
					}
					return nil
				}),
			},
			test.Step{
				Name: "Snapshot repository should be reset",
				Test: test.Eventually(func() error {
					esClient, err := elasticsearch.NewElasticsearchClient(es.Elasticsearch, k)
					assert.NoError(t, err)

					var repos SnapshotRepositories
					err = Request(esClient, http.MethodGet, "/_snapshot", nil, &repos)
					if err != nil {
						return err
					}

					if len(repos) != 0 {
						return errors.New("snapshot repository settings not reset")
					}
					return nil
				}),
			},
		}
	}

	test.Sequence(nil, steps, es).RunSequential(t)
}

type ClusterSettings struct {
	Persistent struct {
		Indices struct {
			Recovery struct {
				MaxBytesPerSec string `json:"max_bytes_per_sec"`
			} `json:"recovery"`
		} `json:"indices"`
	} `json:"persistent"`
}

type SnapshotRepositories map[string]SnapshotRepository

type SnapshotRepository struct {
	Type     string                     `json:"type"`
	Settings SnapshotRepositorySettings `json:"settings"`
}

type SnapshotRepositorySettings struct {
	Bucket   string `json:"bucket"`
	BasePath string `json:"base_path"`
}

// Request is a utility function to call a specific Elasticsearch API not implemented in the Elasticsearch client.
func Request(esClient esClient.Client, method string, url string, body io.Reader, response interface{}) error {
	req, err := http.NewRequest(method, url, body) //nolint:noctx
	if err != nil {
		return err
	}
	resp, err := esClient.Request(context.Background(), req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	resultBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(resultBytes, &response)
	if err != nil {
		return err
	}
	return nil
}
