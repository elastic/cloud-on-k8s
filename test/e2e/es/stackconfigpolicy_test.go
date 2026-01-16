// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

var (
	//go:embed fixtures/stackconfigpolicy_esConfig.yaml
	esConfig string
)

// TestStackConfigPolicy tests the StackConfigPolicy feature.
func TestStackConfigPolicy(t *testing.T) {
	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	// StackConfigPolicy is supported for ES versions with file-based settings feature
	stackVersion := version.MustParse(test.Ctx().ElasticStackVersion)
	if !stackVersion.GTE(filesettings.FileBasedSettingsMinPreVersion) {
		t.SkipNow()
	}

	es := elasticsearch.NewBuilder("test-es-scp").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithLabel("label", "test-scp")

	namespace := test.Ctx().ManagedNamespace(0)
	secureSettingsSecretName := fmt.Sprintf("test-scp-secure-settings-%s", rand.String(4))
	secretMountsSecretName := fmt.Sprintf("test-scp-secret-mounts-%s", rand.String(4))
	clusterNameFromConfig := fmt.Sprintf("test-scp-cluster-%s", rand.String(4))

	// set the policy Elasticsearch settings the policy using the external YAML file
	var esConfigSpec policyv1alpha1.ElasticsearchConfigPolicySpec
	err := yaml.Unmarshal([]byte(esConfig), &esConfigSpec)
	assert.NoError(t, err)

	// list of endpoints to check the existence or not of the settings defined in the esConfigSpec
	configuredObjectsEndpoints := []string{
		"/_snapshot/repo_test",
		"/_slm/policy/slm_test",
		"/_ingest/pipeline/pipeline_test",
		"/_ilm/policy/ilm_test",
		"/_index_template/template_test",
		"/_component_template/runtime_component_template_test",
		"/_component_template/component_template_test",
	}

	esConfigSpec.SecureSettings = []commonv1.SecretSource{
		{SecretName: secureSettingsSecretName},
	}

	esConfigSpec.SecretMounts = []policyv1alpha1.SecretMount{
		{
			SecretName: secretMountsSecretName,
			MountPath:  "/test",
		},
	}

	scpEsConfig := commonv1.NewConfig(map[string]interface{}{"cluster.name": clusterNameFromConfig})
	esConfigSpec.Config = &scpEsConfig

	policy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("test-scp-%s", rand.String(4)),
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			ResourceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"label": "test-scp"},
			},
			Elasticsearch: esConfigSpec,
		},
	}

	secureServiceAccountSettingKey := "gcs.client.secondary.credentials_file"
	secureSettingsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secureSettingsSecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			secureServiceAccountSettingKey: []byte(`{
				"type": "service_account",
				"project_id": "FAKE_PROJECT_ID",
				"private_key_id": "FAKE_KEY_ID",
				"private_key": "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDXgs36nwry0roc\nvJ+yNInGkapcGEiyYVE9hsaZBlHLe9erUsCALtfheMqskcs334tWF8tLqFMG1I2t\nMBfzUEudHqNQ0BP9Z+2ilMXJBhZpNU/i4i7H8LCEjKt34B5K50lJRZ+Lt2Z2zr1c\nZZ9PniPYOaqRLAjwXGwJ6rU1Xt+fJB0EHDni6dS/g6wn498yh81coFW0rSMTdArm\nfvqraf8mL6X7/ElI/RWGsvcjx31wNZv5re2R1sgAZzALac1lYdGhRQFZ73swz4bZ\nktjsZJ0v/38+YPnyELXykqpBYfKTC2n2QO2nFYR8tkfSMhMaSIrFc1YnkVsY8LmX\nZdKafnD5AgMBAAECggEAAjrCMsOOc3CmqEFzTX6ppjo/jvBZYC8Njhtk1pRwKDDB\nzG3wu+LALP746cwgVBWl9WANpFy7byinxpDmzoeYIKn+eomMi2SV2sa7PRcpCDGa\n//fjEAJ3cQebhoP1DEVURsPHoMRm9PeykdAjU8mJCWWfVB0mgoYSQBADi+fNXHIY\nVpZ+GmAg8sVmAMKws2Afa2FFXxAvNnR8wdeqBGhPhYH9yrJAc7c5BuKT/9axVg6u\n622epYd7tmfrQCnyTu1Vzwi0mylDRcC2r+ynl9Hkpr4w4fx3rlrjN/1UVOIcCQli\nHXHr8+n8GPMdDW/mUwZzO0NR2MevBsfZiKO9oCtC2QKBgQDrzuvk112cp3t4YA1I\nYjLHP7aGi+oq14UWrtNGDyS/e6bkbNZW2EkQy0TPcjA0AFe51vj0SE9iv3No1y0R\nsRwvbsg5qydY8SfSKY8Zu1CcfB3f1W1775jnkqx1LCj6LnBC1OIKaCbnAM6d2xxn\nUscNpaAW8+uQMDXtAY3BKkm4CwKBgQDp9vOoilPWVpRew8z5txnSMcwS/2U5sRjJ\nDHem/ZAJXO+4/iQAzPSumlov19fJiAZLD5/NDdJxYM7npQ8+xcZ9DgukPY6T5Qp3\nR0urYNjsEra4Q3A4OWFP7mr+oYQjwnc5slkS4hIafPVBi335Njjlce4DgZtLHIFJ\nwerY1dFpiwKBgFf+t3iGBaDXvvOEpHBGdLx1wh8jRxcFpdx5EM4sCIKMGhNTqghu\nXZWuxNbEvcgp+JKY7f36neUznFWbNm5LsUDiDkW24NAH7dw3NfdcNxCuIFfOxTRi\njKSdz01KVWBGxA2sc01+4EWDv5aYlVjZQv6Mt9jY3SbJVtZCpitXJHtRAoGBANnz\nJjqcWcsyrla1Oe5qRpCLuRr9ddPPiVJI3fHfBd3jCKIhhXKFe25n9ZnaDXf80jf8\nXxYLST47O6OJHPGSFfyLKAchHP/i/uPss637szf/mt1+XTzTHzbx2BRKbClPz/cc\nkGPJ26l3PJWJl5mfjFMZ1erIQt0uubX3AopqbQFPAoGAFsAMpU7OE/VG4/BLEufp\n7XEYhU5UiA4qsFKxobZpek7wQMAw3e3qrsyh3mD0D3qTl6Jq+YXW9c62kEny67mG\nW6v/s3KPbYdWDSf3R3t0Wx1Ym9QMT+oxUEjO9fZ79Gfa17XC9xH6Uqkn7FakzzPX\nTQI222EehboxE6Cys4/usKg=\n-----END PRIVATE KEY-----\n",
				"client_email": "FAKE_SERVICE_ACCOUNT_EMAIL",
				"client_id": "CLIENT_ID",
				"auth_uri": "https://accounts.google.com/o/oauth2/auth",
				"token_uri": "https://accounts.google.com/o/oauth2/token",
				"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
				"client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/SERVICE_ACCOUNT_EMAIL"
        	}`),
		},
	}

	secretsMountSecretKey := "testfile"
	secretMountsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretMountsSecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			secretsMountSecretKey: []byte(`testfile content`),
		},
	}

	esWithlicense := test.LicenseTestBuilder(es)

	var noEntries []string
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
				Name: "Create a Secret Mounts secret",
				Test: test.Eventually(func() error {
					err := k.CreateOrUpdate(&secretMountsSecret)
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
					_, _, err = request(esClient, http.MethodGet, "/_cluster/settings", nil, &settings)
					if err != nil {
						return err
					}

					if settings.Persistent.Indices.Recovery.MaxBytesPerSec != "100mb" {
						return errors.New("cluster settings not configured")
					}
					return nil
				}),
			},
			test.Step{
				Name: "Cluster name should be as set in the config",
				Test: test.Eventually(func() error {
					esClient, err := elasticsearch.NewElasticsearchClient(es.Elasticsearch, k)
					assert.NoError(t, err)

					var apiResponse ClusterInfoResponse
					if _, _, err = request(esClient, http.MethodGet, "/", nil, &apiResponse); err != nil {
						return err
					}

					require.Equal(t, clusterNameFromConfig, apiResponse.ClusterName)
					return nil
				}),
			},
			test.Step{
				Name: "Snapshot repository should be configured",
				Test: test.Eventually(func() error {
					esClient, err := elasticsearch.NewElasticsearchClient(es.Elasticsearch, k)
					assert.NoError(t, err)

					repoName := "repo_test"
					var repos SnapshotRepositories
					_, _, err = request(esClient, http.MethodGet, filepath.Join("/_snapshot", repoName), nil, &repos)
					if err != nil {
						return err
					}

					actualRepo, ok := repos[repoName]
					if !ok {
						return fmt.Errorf("snapshot repository '%s' not found", repoName)
					}
					expectedRepo := SnapshotRepository{
						Type: "gcs",
						Settings: SnapshotRepositorySettings{
							Bucket:   "my-bucket",
							BasePath: fmt.Sprintf("snapshots/%s-%s", es.Namespace(), es.Name())},
					}
					if !reflect.DeepEqual(actualRepo, expectedRepo) {
						act, err := json.Marshal(actualRepo)
						if err != nil {
							return err
						}
						exp, err := json.Marshal(expectedRepo)
						if err != nil {
							return err
						}
						return fmt.Errorf("snapshot repository not configured: expected: %s, actual: %s", act, exp)
					}
					return nil
				}),
			},
			test.Step{
				Name: "Role mappings should be set",
				Test: test.Eventually(func() error {
					esClient, err := elasticsearch.NewElasticsearchClient(es.Elasticsearch, k)
					assert.NoError(t, err)

					metadataUUID := "b9a59ba9-6b92-4be2-bb8d-02bb270cb3a7" // from test/e2e/es/fixtures/stackconfigpolicy_esConfig.yaml

					// except in 8.15.x due to a bug, role mappings are exposed via the API
					if stackVersion.LT(version.MinFor(8, 15, 0)) && stackVersion.GTE(version.MinFor(8, 16, 0)) {
						if err := checkAPIResponse(esClient, "/_security/role_mapping/role_test", 200, metadataUUID); err != nil {
							return err
						}
					}
					// starting 8.15.x, role mappings are stored in the cluster state
					if stackVersion.GTE(version.MinFor(8, 15, 0)) {
						if err := checkAPIResponse(esClient, "/_cluster/state/metadata?filter_path=metadata.role_mappings", 200, metadataUUID); err != nil {
							return err
						}
					}

					return nil
				}),
			},
			test.Step{
				Name: "Other settings should be set",
				Test: test.Eventually(func() error {
					esClient, err := elasticsearch.NewElasticsearchClient(es.Elasticsearch, k)
					assert.NoError(t, err)

					for _, ep := range configuredObjectsEndpoints {
						if err := checkAPIStatusCode(esClient, ep, 200); err != nil {
							return err
						}
					}

					return nil
				}),
			},
			elasticsearch.CheckESKeystoreEntries(k, es, []string{
				secureServiceAccountSettingKey,
			}),
			elasticsearch.CheckStackConfigPolicyESSecretMountsVolume(k, es.Elasticsearch, policy),
			test.Step{
				Name: "Deleting the StackConfigPolicy should return no error",
				Test: test.Eventually(func() error {
					return k.Client.Delete(context.Background(), &policy)
				}),
			},
			test.Step{
				Name: "Settings should be reset",
				Test: test.Eventually(func() error {
					esClient, err := elasticsearch.NewElasticsearchClient(es.Elasticsearch, k)
					assert.NoError(t, err)

					for _, ep := range configuredObjectsEndpoints {
						if err := checkAPIStatusCode(esClient, ep, 404); err != nil {
							return err
						}
					}
					return nil
				}),
			},
			test.Step{
				Name: "Role mappings should be reset",
				Test: test.Eventually(func() error {
					esClient, err := elasticsearch.NewElasticsearchClient(es.Elasticsearch, k)
					assert.NoError(t, err)

					// starting 8.15.x, role mappings are correctly removed
					if stackVersion.GTE(version.MinFor(8, 15, 0)) {
						if err := checkAPIStatusCode(esClient, "/_security/role_mapping/role_test", 404); err != nil {
							return err
						}
						if err := checkAPIResponse(esClient, "/_cluster/state/metadata?filter_path=metadata.role_mappings", 200, "{}"); err != nil {
							return err
						}
					}

					return nil
				}),
			},
			// keystore entries should be removed
			elasticsearch.CheckESKeystoreEntries(k, es, noEntries),
			test.Step{
				Name: "Delete secure settings secret",
				Test: test.Eventually(func() error {
					return k.Client.Delete(context.Background(), &secureSettingsSecret)
				}),
			},
			test.Step{
				Name: "Delete secure mounts secret",
				Test: test.Eventually(func() error {
					return k.Client.Delete(context.Background(), &secretMountsSecret)
				}),
			},
		}
	}

	test.Sequence(nil, steps, esWithlicense).RunSequential(t)
}

// TestStackConfigPolicyMultipleWeights tests multiple StackConfigPolicies with different weights.
func TestStackConfigPolicyMultipleWeights(t *testing.T) {
	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	stackVersion := version.MustParse(test.Ctx().ElasticStackVersion)
	switch {
	case stackVersion.LT(filesettings.FileBasedSettingsMinPreVersion):
		// StackConfigPolicy is supported for ES versions with file-based settings feature
		t.SkipNow()
	case stackVersion.LT(version.From(8, 11, 0)):
		// Before 8.11.0, ES has an issue with loading cluster-settings changes in file-settings
		// of the same keys as in this test (https://github.com/elastic/elasticsearch/pull/99212)
		t.SkipNow()
	}

	es := elasticsearch.NewBuilder("test-es-scp-multi").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithLabel("app", "elasticsearch")

	namespace := test.Ctx().ManagedNamespace(0)

	// Policy with weight 20 (lower priority) - sets cluster.name
	lowPriorityPolicy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("low-priority-scp-%s", rand.String(4)),
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			Weight: 20,
			ResourceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "elasticsearch"},
			},
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"cluster.name": "low-priority-cluster",
					},
				},
				ClusterSettings: &commonv1.Config{
					Data: map[string]interface{}{
						"indices": map[string]interface{}{
							"recovery.max_bytes_per_sec": "50mb",
						},
					},
				},
			},
		},
	}

	// Policy with weight 10 (higher priority) - should override cluster.name and settings
	highPriorityPolicy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("high-priority-scp-%s", rand.String(4)),
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			Weight: 10,
			ResourceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "elasticsearch"},
			},
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"cluster": map[string]interface{}{
							"name": "high-priority-cluster",
						},
					},
				},
				ClusterSettings: &commonv1.Config{
					Data: map[string]interface{}{
						"indices": map[string]interface{}{
							"recovery": map[string]interface{}{
								"max_bytes_per_sec": "200mb",
							},
						},
					},
				},
			},
		},
	}

	// Policy with same weight 20 but different selector (should not conflict)
	nonConflictingPolicy := policyv1alpha1.StackConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("non-conflicting-scp-%s", rand.String(4)),
		},
		Spec: policyv1alpha1.StackConfigPolicySpec{
			Weight: 20,
			ResourceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "kibana"}, // Different selector
			},
			Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"cluster.name": "should-not-apply",
					},
				},
			},
		},
	}

	esWithlicense := test.LicenseTestBuilder(es)

	steps := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			test.Step{
				Name: "Create low priority StackConfigPolicy",
				Test: test.Eventually(func() error {
					return k.CreateOrUpdate(&lowPriorityPolicy)
				}),
			},
			test.Step{
				Name: "Create high priority StackConfigPolicy",
				Test: test.Eventually(func() error {
					return k.CreateOrUpdate(&highPriorityPolicy)
				}),
			},
			test.Step{
				Name: "Create non-conflicting StackConfigPolicy",
				Test: test.Eventually(func() error {
					return k.CreateOrUpdate(&nonConflictingPolicy)
				}),
			},
			test.Step{
				Name: "High priority cluster name should be applied",
				Test: test.Eventually(func() error {
					esClient, err := elasticsearch.NewElasticsearchClient(es.Elasticsearch, k)
					if err != nil {
						return err
					}

					var apiResponse ClusterInfoResponse
					if _, _, err = request(esClient, http.MethodGet, "/", nil, &apiResponse); err != nil {
						return err
					}

					if apiResponse.ClusterName != "high-priority-cluster" {
						return fmt.Errorf("expected cluster name 'high-priority-cluster', got '%s'", apiResponse.ClusterName)
					}
					return nil
				}),
			},
			test.Step{
				Name: "High priority cluster settings should be applied",
				Test: test.Eventually(func() error {
					esClient, err := elasticsearch.NewElasticsearchClient(es.Elasticsearch, k)
					if err != nil {
						return err
					}

					var settings ClusterSettings
					_, _, err = request(esClient, http.MethodGet, "/_cluster/settings", nil, &settings)
					if err != nil {
						return err
					}

					if settings.Persistent.Indices.Recovery.MaxBytesPerSec != "200mb" {
						return fmt.Errorf("expected max_bytes_per_sec '200mb', got '%s'", settings.Persistent.Indices.Recovery.MaxBytesPerSec)
					}
					return nil
				}),
			},
			test.Step{
				Name: "Delete high priority policy - low priority should take effect",
				Test: test.Eventually(func() error {
					return k.Client.Delete(context.Background(), &highPriorityPolicy)
				}),
			},
			test.Step{
				Name: "Low priority cluster name should now be applied",
				Test: test.Eventually(func() error {
					esClient, err := elasticsearch.NewElasticsearchClient(es.Elasticsearch, k)
					if err != nil {
						return err
					}

					var apiResponse ClusterInfoResponse
					if _, _, err = request(esClient, http.MethodGet, "/", nil, &apiResponse); err != nil {
						return err
					}

					if apiResponse.ClusterName != "low-priority-cluster" {
						return fmt.Errorf("expected cluster name 'low-priority-cluster', got '%s'", apiResponse.ClusterName)
					}
					return nil
				}),
			},
			test.Step{
				Name: "Low priority cluster settings should now be applied",
				Test: test.Eventually(func() error {
					esClient, err := elasticsearch.NewElasticsearchClient(es.Elasticsearch, k)
					if err != nil {
						return err
					}

					var settings ClusterSettings
					_, _, err = request(esClient, http.MethodGet, "/_cluster/settings", nil, &settings)
					if err != nil {
						return err
					}

					if settings.Persistent.Indices.Recovery.MaxBytesPerSec != "50mb" {
						return fmt.Errorf("expected max_bytes_per_sec '50mb', got '%s'", settings.Persistent.Indices.Recovery.MaxBytesPerSec)
					}
					return nil
				}),
			},
			test.Step{
				Name: "Clean up remaining policies",
				Test: test.Eventually(func() error {
					if err := k.Client.Delete(context.Background(), &lowPriorityPolicy); err != nil {
						return err
					}
					return k.Client.Delete(context.Background(), &nonConflictingPolicy)
				}),
			},
		}
	}

	test.Sequence(nil, steps, esWithlicense).RunSequential(t)
}

func checkAPIStatusCode(esClient client.Client, url string, expectedStatusCode int) error {
	var items map[string]interface{}
	_, actualStatusCode, _ := request(esClient, http.MethodGet, url, nil, &items)
	if expectedStatusCode != actualStatusCode {
		return fmt.Errorf("calling %s should return %d, got %d", url, expectedStatusCode, actualStatusCode)
	}
	return nil
}

func checkAPIResponse(esClient client.Client, url string, expectedStatusCode int, expectedResponse string) error {
	var items map[string]interface{}
	response, actualStatusCode, _ := request(esClient, http.MethodGet, url, nil, &items)
	if expectedStatusCode != actualStatusCode {
		return fmt.Errorf("calling %s should return %d, got %d", url, expectedStatusCode, actualStatusCode)
	}
	if !strings.Contains(string(response), expectedResponse) {
		return fmt.Errorf("calling %s should contain [%s] in [%s]", url, expectedResponse, string(response))
	}
	return nil
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

type ClusterInfoResponse struct {
	ClusterName string `json:"cluster_name"`
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

// request is a utility function to call a specific Elasticsearch API not implemented in the Elasticsearch client.
func request(esClient client.Client, method string, url string, body io.Reader, response interface{}) ([]byte, int, error) {
	req, err := http.NewRequest(method, url, body) //nolint:noctx
	statusCode := 0
	if err != nil {
		return nil, statusCode, err
	}
	resp, err := esClient.Request(context.Background(), req)
	if resp != nil {
		statusCode = resp.StatusCode
	}
	if err != nil {
		return nil, statusCode, err
	}
	defer resp.Body.Close()
	resultBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, statusCode, err
	}
	err = json.Unmarshal(resultBytes, &response)
	if err != nil {
		return resultBytes, statusCode, err
	}
	return resultBytes, statusCode, nil
}
