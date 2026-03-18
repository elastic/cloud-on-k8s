// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package shared

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/network"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/services"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	commondriver "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	commonversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	watches2 "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	client2 "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/cryptutil"

	"github.com/stretchr/testify/assert"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_esReachableConditionMessage(t *testing.T) {
	type args struct {
		internalService        *corev1.Service
		isServiceReady         bool
		isRespondingToRequests bool
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			args: args{
				internalService:        &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "namespace"}},
				isServiceReady:         false,
				isRespondingToRequests: false,
			},
			want: "Service namespace/name has no endpoint",
		},
		{
			args: args{
				internalService:        &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "namespace"}},
				isServiceReady:         true,
				isRespondingToRequests: false,
			},
			want: "Service namespace/name has endpoints but Elasticsearch is unavailable",
		},
		{
			args: args{
				internalService:        &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "namespace"}},
				isServiceReady:         true,
				isRespondingToRequests: true,
			},
			want: "Service namespace/name has endpoints",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := esReachableConditionMessage(tt.args.internalService, tt.args.isServiceReady, tt.args.isRespondingToRequests); got != tt.want {
				t.Errorf("EsReachableConditionMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_maybeReconcileEmptyFileSettingsSecret(t *testing.T) {
	const operatorNamespace = "elastic-system"

	tests := []struct {
		name              string
		es                *esv1.Elasticsearch
		policies          []policyv1alpha1.StackConfigPolicy
		existingSecrets   []corev1.Secret
		licenseChecker    commonlicense.Checker
		wantSecretCreated bool
		wantRequeue       bool
		wantErr           bool
	}{
		{
			name: "No policies exist - should create empty secret (enterprise enabled)",
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "default",
					Labels: map[string]string{
						"app": "elasticsearch",
					},
				},
			},
			licenseChecker:    commonlicense.MockLicenseChecker{EnterpriseEnabled: true},
			wantSecretCreated: true,
			wantRequeue:       false,
			wantErr:           false,
		},
		{
			name: "No policies exist - should create empty secret (enterprise disabled)",
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "default",
					Labels: map[string]string{
						"app": "elasticsearch",
					},
				},
			},
			licenseChecker:    commonlicense.MockLicenseChecker{EnterpriseEnabled: false},
			wantSecretCreated: true,
			wantRequeue:       false,
			wantErr:           false,
		},
		{
			name: "Policy targets ES cluster in same namespace - should NOT create empty secret but requeue",
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "default",
					Labels: map[string]string{
						"app": "elasticsearch",
					},
				},
			},
			policies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-policy",
						Namespace: "default",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "elasticsearch",
							},
						},
					},
				},
			},
			licenseChecker:    commonlicense.MockLicenseChecker{EnterpriseEnabled: true},
			wantSecretCreated: false,
			wantRequeue:       true,
			wantErr:           false,
		},
		{
			name: "Policy targets ES cluster from operator namespace - should NOT create empty secret but requeue",
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "default",
					Labels: map[string]string{
						"app": "elasticsearch",
					},
				},
			},
			policies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "global-policy",
						Namespace: "elastic-system",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "elasticsearch",
							},
						},
					},
				},
			},
			licenseChecker:    commonlicense.MockLicenseChecker{EnterpriseEnabled: true},
			wantSecretCreated: false,
			wantRequeue:       true,
			wantErr:           false,
		},
		{
			name: "Policy exists but does not target ES cluster - should create empty secret",
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "default",
					Labels: map[string]string{
						"app": "elasticsearch",
					},
				},
			},
			policies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unrelated-policy",
						Namespace: "default",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "other-app",
							},
						},
					},
				},
			},
			licenseChecker:    commonlicense.MockLicenseChecker{EnterpriseEnabled: true},
			wantSecretCreated: true,
			wantRequeue:       false,
			wantErr:           false,
		},
		{
			name: "Policy in different namespace (not operator namespace) - should create empty secret",
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "default",
					Labels: map[string]string{
						"app": "elasticsearch",
					},
				},
			},
			policies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-ns-policy",
						Namespace: "other-namespace",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "elasticsearch",
							},
						},
					},
				},
			},
			licenseChecker:    commonlicense.MockLicenseChecker{EnterpriseEnabled: true},
			wantSecretCreated: true,
			wantRequeue:       false,
			wantErr:           false,
		},
		{
			name: "Multiple policies, one targets ES, file-settings secret does not exist - should NOT create empty secret but requeue",
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "default",
					Labels: map[string]string{
						"app":  "elasticsearch",
						"team": "platform",
					},
				},
			},
			policies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unrelated-policy",
						Namespace: "default",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "other-app",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "matching-policy",
						Namespace: "default",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"team": "platform",
							},
						},
					},
				},
			},
			licenseChecker:    commonlicense.MockLicenseChecker{EnterpriseEnabled: true},
			wantSecretCreated: false,
			wantRequeue:       true,
			wantErr:           false,
		},
		{
			name: "Multiple policies, one targets ES, file-settings secret exists - should NOT requeue",
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "default",
					Labels: map[string]string{
						"app":  "elasticsearch",
						"team": "platform",
					},
				},
			},
			existingSecrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      esv1.FileSettingsSecretName("test-es"),
						Namespace: "default",
					},
				},
			},
			policies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unrelated-policy",
						Namespace: "default",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "other-app",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "matching-policy",
						Namespace: "default",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"team": "platform",
							},
						},
					},
				},
			},
			licenseChecker:    commonlicense.MockLicenseChecker{EnterpriseEnabled: true},
			wantSecretCreated: true,
			wantRequeue:       false,
			wantErr:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with initial objects
			var initObjs []client.Object
			for i := range tt.policies {
				initObjs = append(initObjs, &tt.policies[i])
			}
			for i := range tt.existingSecrets {
				initObjs = append(initObjs, &tt.existingSecrets[i])
			}
			initObjs = append(initObjs, tt.es)

			c := k8s.NewFakeClient(initObjs...)

			requeue, err := maybeReconcileEmptyFileSettingsSecret(t.Context(), c, tt.licenseChecker, tt.es, operatorNamespace)

			// Check error expectation
			if tt.wantErr {
				assert.Error(t, err, "expected error at maybeReconcileEmptyFileSettingsSecret")
				return
			}
			assert.NoError(t, err, "expected no error at maybeReconcileEmptyFileSettingsSecret")
			assert.Equal(t, tt.wantRequeue, requeue, "expected requeue does not match")

			// Check if secret was created
			var secret corev1.Secret
			secretName := esv1.FileSettingsSecretName(tt.es.Name)
			secretErr := c.Get(t.Context(), types.NamespacedName{
				Name:      secretName,
				Namespace: tt.es.Namespace,
			}, &secret)

			if tt.wantSecretCreated {
				assert.NoError(t, secretErr, "expected no error at getting file-settings secret")
			} else {
				assert.True(t, apierrors.IsNotFound(secretErr), "expected IsNotFound error at getting file-settings secret")
			}
		})
	}
}

var standardElasticsearch = esv1.Elasticsearch{
	ObjectMeta: metav1.ObjectMeta{
		Name: "test-es", Namespace: "test-ns",
		ResourceVersion: "1",
		Annotations:     map[string]string{observer.ObserverIntervalAnnotation: "10s"},
		Labels:          map[string]string{label.VersionLabelName: "9.3.1", label.ClusterNameLabelName: "test-es"},
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: "operator.elastic.co/v1",
				Kind:       "Elasticsearch",
				Name:       "test-es",
				UID:        "test-es-uid",
			},
		},
	},
	Spec: esv1.ElasticsearchSpec{
		HTTP: commonv1.HTTPConfig{
			Service: commonv1.ServiceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es-es-internal-http",
					Namespace: "test-ns",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Name: "http", Port: 9200}},
				}},
		},
		Version: "9.3.1"},
}

type serviceType int

const (
	transport serviceType = iota
	external
	internal
	remote
)

func TestReconcileSharedResources(t *testing.T) {
	const namespace = "test-ns"
	const clusterName = "test-es"

	esServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data string
		switch r.URL.Path {
		case "/_cluster/health":
			data = `{"cluster_name" : "test-es","status" : "green","timed_out" : false,"number_of_nodes" : 1,"number_of_data_nodes" : 1,"active_primary_shards" : 1,"active_shards" : 1,"relocating_shards" : 0,"initializing_shards" : 0,"unassigned_shards" : 1,"delayed_unassigned_shards": 0,"number_of_pending_tasks" : 0,"number_of_in_flight_fetch": 0,"task_max_waiting_in_queue_millis": 0,"active_shards_percent_as_number": 50.0}`
		case "/_license":
			data = `{"license": {"status": "active","uid": "...","type": "basic","version": 1,"issue_date": "2024-01-01T00:00:00.000Z","issue_date_in_millis": 1704067200000,"expiry_date": "2099-12-31T23:59:59.999Z","expiry_date_in_millis": 4102358399999,"max_nodes": 1000,"issued_to": "issuedTo","issuer": "issuer","signature": "...","start_date_in_millis": 1704067200000}}`
		default:
			data = `{"cluster_name":"test-es","cluster_uuid":"abc123","version":{"number":"9.3.1"}, "tagline":"You Know, for Search"}`
		}

		_, err := w.Write([]byte(data))
		require.NoError(t, err)
	}))
	defer esServer.Close()

	tests := []struct {
		name                   string
		params                 driver.Parameters
		expectedState          *ReconcileState
		reconciliationExpected bool
		expectedServices       map[string]corev1.Service
		expectedSecrets        map[string]corev1.Secret
		expectedConfigMaps     map[string]corev1.ConfigMap
	}{
		{
			name: "happy path - new Elasticsearch with no remote cluster",
			params: mustGetParams(t, esServer, 0,
				&standardElasticsearch,
				getPod(&standardElasticsearch, esServer.Listener.Addr()),
			),
			expectedServices: map[string]corev1.Service{
				esv1.TransportService(clusterName):        getExpectedService(&standardElasticsearch, transport, "1"),
				services.ExternalServiceName(clusterName): getExpectedService(&standardElasticsearch, external, "1"),
				services.InternalServiceName(clusterName): getExpectedService(&standardElasticsearch, internal, "1"),
			},
			expectedSecrets:    getExpectedSecrets(&standardElasticsearch, "1"),
			expectedConfigMaps: mustGetExpectedConfigMaps(t, &standardElasticsearch, "1"),
			expectedState: &ReconcileState{
				Meta: metadata.Metadata{
					Labels: map[string]string{
						label.ClusterNameLabelName:   clusterName,
						"common.k8s.elastic.co/type": "elasticsearch",
					},
					Annotations: nil,
				},
				ESReachable: true,
			},
			reconciliationExpected: true,
		},
		// TODO Add more test cases. Right now there's only "happy path". Cases like no endpoints, missing pods, or
		//version mismatches would make this test much more useful as a safety net for the upcoming reconciliation
		//refactoring.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := tt.params.Client
			testDriver := commondriver.TestDriver{
				Client:       k8sClient,
				Watches:      tt.params.DynamicWatches,
				FakeRecorder: record.NewFakeRecorder(1000),
			}

			s, results := ReconcileSharedResources(context.Background(), testDriver, tt.params)
			assert.EqualValues(t, tt.expectedState.ESReachable, s.ESReachable)
			assert.EqualValues(t, tt.expectedState.KeystoreResources, s.KeystoreResources)
			actualIsReconciled, _ := results.IsReconciled()
			assert.EqualValues(t, tt.reconciliationExpected, actualIsReconciled, "Expected reconciliation to be %v, got %v", tt.reconciliationExpected, actualIsReconciled)

			// Ensure expected secrets are created
			actualSecrets := &corev1.SecretList{}
			assert.NoError(t, k8sClient.List(context.Background(), actualSecrets, client.InNamespace(namespace)))
			assert.Len(t, actualSecrets.Items, len(tt.expectedSecrets), "Unexpected number of secrets created")
			for _, secret := range actualSecrets.Items {
				_, ok := tt.expectedSecrets[secret.Name]
				assert.Truef(t, ok, "Unexpected secret [%s] created", secret.Name)
				//assert.Equalf(t, expectedSecret, secret, "secret [%s] did not match expectations", secret.Name)
			}

			// Ensure expected ES client is created
			assert.NotNil(t, s.ESClient, "Expected non-nil ES client")
			// HasProperties inherently asserts expected certificates, user credentials, URL, and version
			//expectedCACerts := getHTTPCACerts(tt.expectedSecrets)
			//assert.True(t, s.ESClient.HasProperties(mustParseVersion(t, "9.3.1"), client2.BasicAuth{Name: user.ControllerUserName, Password: staticPassword}, tt.params.URLProvider, expectedCACerts[:3]), "Generated Elasticsearch client does not have expected properties")

			// Ensure expected services are created
			actualServices := &corev1.ServiceList{}
			assert.NoError(t, k8sClient.List(context.Background(), actualServices, client.InNamespace(namespace)))
			assert.Len(t, actualServices.Items, len(tt.expectedServices), "Unexpected number of services created")
			for _, svc := range actualServices.Items {
				expectedService, ok := tt.expectedServices[svc.Name]
				assert.Truef(t, ok, "Unexpected service [%s] created", svc.Name)
				assert.Equalf(t, expectedService, svc, "service [%s] does not match expected value", svc.Name)
			}

			// Ensure expected ConfigMaps are created
			actualConfigMaps := &corev1.ConfigMapList{}
			assert.NoError(t, k8sClient.List(context.Background(), actualConfigMaps, client.InNamespace(namespace)))
			assert.Len(t, actualConfigMaps.Items, len(tt.expectedConfigMaps), "Unexpected number of ConfigMaps created")
			for _, cm := range actualConfigMaps.Items {
				expectedConfigMap, ok := tt.expectedConfigMaps[cm.Name]
				assert.Truef(t, ok, "Unexpected ConfigMap [%s] created", cm.Name)
				assert.Equalf(t, expectedConfigMap, cm, "ConfigMap [%s] does not match expected value", cm.Name)
			}
		})
	}
}

func getHTTPCACerts(secrets map[string]corev1.Secret) []*x509.Certificate {
	certs := make([]*x509.Certificate, 0, 3) // public, internal, internal-ca
	for name, secret := range secrets {
		if strings.Contains(name, "http") {
			for k, v := range secret.Data {
				if k == "ca.crt" {
					certs = append(certs, &x509.Certificate{Raw: v})
				}
			}
		}
	}

	return certs
}

func getServiceAccountTokenSecret(es *esv1.Elasticsearch) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-es-token-xxxxx", es.Name),
			Namespace: es.Namespace,
			Labels: map[string]string{
				label.ClusterNameLabelName: es.Name,
				commonv1.TypeLabelName:     user.ServiceAccountTokenType,
			},
		},
		Type: corev1.SecretTypeServiceAccountToken,
		Data: map[string][]byte{
			user.ServiceAccountTokenNameField: []byte("autogenerated"),
			user.ServiceAccountHashField:      []byte("autogenerated"),
		},
	}
}

func generateRemoteCAs(namespace, name string, quantity int) []client.Object {
	secretsToCreate := make([]client.Object, quantity)
	for i := 0; i < quantity; i++ {
		secretsToCreate[i] = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("remote-ca-%s-%d", name, i),
				Namespace: namespace,
				Labels: map[string]string{
					label.ClusterNameLabelName:   name,
					"common.k8s.elastic.co/type": "remote-ca",
				},
			},
			Data: map[string][]byte{},
		}
	}

	return secretsToCreate
}

func mustGetExpectedConfigMaps(t *testing.T, es *esv1.Elasticsearch, resourceVersion string) map[string]corev1.ConfigMap {
	labels := getLabels(es)
	ownerReferences := getOwnerReferences(es)

	fsScript, err := initcontainer.RenderPrepareFsScript(es.DownwardNodeLabels())
	require.NoError(t, err, "error rendering FS script")
	preStopScript, err := nodespec.RenderPreStopHookScript(services.InternalServiceURL(*es))
	require.NoError(t, err, "error rendering preStop script")

	return map[string]corev1.ConfigMap{
		esv1.ScriptsConfigMap(es.Name): {
			ObjectMeta: metav1.ObjectMeta{
				Name:            esv1.ScriptsConfigMap(es.Name),
				Namespace:       es.Namespace,
				ResourceVersion: resourceVersion,
				Labels:          labels,
				OwnerReferences: ownerReferences,
			},
			Data: map[string]string{
				nodespec.LegacyReadinessProbeScriptConfigKey: nodespec.LegacyReadinessProbeScript,
				nodespec.ReadinessPortProbeScriptConfigKey:   nodespec.ReadinessPortProbeScript,
				nodespec.PreStopHookScriptConfigKey:          preStopScript,
				initcontainer.PrepareFsScriptConfigKey:       fsScript,
				initcontainer.SuspendScriptConfigKey:         initcontainer.SuspendScript,
				initcontainer.SuspendedHostsFile:             initcontainer.RenderSuspendConfiguration(*es),
			}},
		esv1.UnicastHostsConfigMap(es.Name): {
			ObjectMeta: metav1.ObjectMeta{
				Name:            esv1.UnicastHostsConfigMap(es.Name),
				Namespace:       es.Namespace,
				ResourceVersion: resourceVersion,
				Labels:          labels,
				OwnerReferences: ownerReferences,
			},
			Data: map[string]string{
				"unicast_hosts.txt": "127.0.0.1:9300",
			},
		},
	}
}

func getExpectedSecrets(es *esv1.Elasticsearch, resourceVersion string) map[string]corev1.Secret {
	// 9 baseline
	labels := getLabels(es)
	labels["eck.k8s.elastic.co/credentials"] = "true"
	labels["eck.k8s.elastic.co/owner-kind"] = ""
	labels["eck.k8s.elastic.co/owner-name"] = es.Name
	labels["eck.k8s.elastic.co/owner-namespace"] = es.Namespace

	serviceAccountTokenSecret := getServiceAccountTokenSecret(es)

	return map[string]corev1.Secret{
		serviceAccountTokenSecret.Name: *serviceAccountTokenSecret,
		fmt.Sprintf("%s-es-transport-ca-internal", es.Name): {
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("%s-es-transport-ca-internal", es.Name),
				Namespace:       es.Namespace,
				ResourceVersion: resourceVersion,
				Labels:          labels,
			},
			Data: map[string][]byte{
				"tls.crt": []byte("autogenerated"),
				"tls.key": []byte("autogenerated"),
			},
		},
		fmt.Sprintf("%s-es-transport-certs-public", es.Name): {
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("%s-es-transport-certs-public", es.Name),
				Namespace:       es.Namespace,
				ResourceVersion: resourceVersion,
				Labels:          labels,
			},
			Data: map[string][]byte{
				"ca.crt": []byte("autogenerated"),
			},
		},
		fmt.Sprintf("%s-es-http-ca-internal", es.Name): {
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("%s-es-http-ca-internal", es.Name),
				Namespace:       es.Namespace,
				ResourceVersion: resourceVersion,
				Labels:          labels,
			},
			Data: map[string][]byte{
				"tls.crt": []byte("autogenerated"),
				"tls.key": []byte("autogenerated"),
			},
		},
		fmt.Sprintf("%s-es-http-certs-internal", es.Name): {
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("%s-es-http-certs-internal", es.Name),
				Namespace:       es.Namespace,
				ResourceVersion: resourceVersion,
				Labels:          labels,
			},
			Data: map[string][]byte{
				"ca.crt":  []byte("autogenerated"),
				"tls.crt": []byte("autogenerated"),
				"tls.key": []byte("autogenerated"),
			},
		},
		fmt.Sprintf("%s-es-http-certs-public", es.Name): {
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("%s-es-http-certs-public", es.Name),
				Namespace:       es.Namespace,
				ResourceVersion: resourceVersion,
				Labels:          labels,
			},
			Data: map[string][]byte{
				"ca.crt":  []byte("autogenerated"),
				"tls.crt": []byte("autogenerated"),
			},
		},
		fmt.Sprintf("%s-es-remote-ca", es.Name): {
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("%s-es-es-remote-ca", es.Name),
				Namespace:       es.Namespace,
				ResourceVersion: resourceVersion,
				Labels:          labels,
			},
			Data: map[string][]byte{
				"ca.crt": []byte("autogenerated"),
			},
		},
		esv1.RolesAndFileRealmSecret(es.Name): {
			ObjectMeta: metav1.ObjectMeta{
				Name:            esv1.RolesAndFileRealmSecret(es.Name),
				Namespace:       es.Namespace,
				ResourceVersion: resourceVersion,
				Labels:          labels,
			},
			Data: map[string][]byte{
				"users":          []byte("autogenerated"),
				"users_roles":    []byte("autogenerated"),
				"roles.yml":      []byte("autogenerated"),
				"service_tokens": []byte("autogenerated"),
			},
		},
		esv1.InternalUsersSecret(es.Name): {
			ObjectMeta: metav1.ObjectMeta{
				Name:            esv1.InternalUsersSecret(es.Name),
				Namespace:       es.Namespace,
				ResourceVersion: resourceVersion,
				Labels:          labels,
			},
			Data: map[string][]byte{
				user.ControllerUserName:  []byte(staticPassword),
				user.PreStopUserName:     []byte(staticPassword),
				user.ProbeUserName:       []byte(staticPassword),
				user.MonitoringUserName:  []byte(staticPassword),
				user.DiagnosticsUserName: []byte(staticPassword),
			},
		},
		esv1.ElasticUserSecret(es.Name): {
			ObjectMeta: metav1.ObjectMeta{
				Name:            esv1.ElasticUserSecret(es.Name),
				Namespace:       es.Namespace,
				ResourceVersion: resourceVersion,
				Labels:          labels,
			},
			Data: map[string][]byte{
				user.ElasticUserName: []byte(staticPassword),
			},
		},
	}
}

func getExpectedService(es *esv1.Elasticsearch, st serviceType, resourceVersion string) corev1.Service {
	serviceName := getServiceName(st, es.Name)
	publishNotReadyAddresses := false
	clusterIP := ""
	if st == transport || st == remote {
		clusterIP = corev1.ClusterIPNone
		publishNotReadyAddresses = true
	}

	labels := getLabels(es)

	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            serviceName,
			Namespace:       es.Namespace,
			Labels:          labels,
			ResourceVersion: resourceVersion,
			OwnerReferences: getOwnerReferences(es),
		},
		Spec: corev1.ServiceSpec{
			Ports:                    []corev1.ServicePort{getPortForService(st)},
			Selector:                 labels,
			ClusterIP:                clusterIP,
			Type:                     corev1.ServiceTypeClusterIP,
			PublishNotReadyAddresses: publishNotReadyAddresses,
		},
	}
}

func getPod(es *esv1.Elasticsearch, addr net.Addr) *corev1.Pod {
	ip := strings.Split(addr.String(), ":")[0]
	podName := fmt.Sprintf("%s-%s", es.Name, uuid.NewUUID()[:6])
	statefulSetName := es.Labels[label.StatefulSetNameLabelName]
	labels := getLabels(es)
	labels[string(label.NodeTypesMasterLabelName)] = "true"
	labels[label.VersionLabelName] = es.Labels[label.VersionLabelName]
	return &corev1.Pod{
		Status: corev1.PodStatus{
			PodIP: ip,
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Status: corev1.ConditionTrue,
					Type:   corev1.ContainersReady},
				{
					Status: corev1.ConditionTrue,
					Type:   corev1.PodReady,
				},
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: es.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			HostAliases: []corev1.HostAlias{
				{
					IP:        ip,
					Hostnames: []string{fmt.Sprintf("%s.%s.%s", podName, statefulSetName, es.Namespace)},
				},
			},
		},
	}
}

func getLabels(es *esv1.Elasticsearch) map[string]string {
	return map[string]string{
		label.ClusterNameLabelName:   es.Name,
		"common.k8s.elastic.co/type": "elasticsearch",
	}
}

func getOwnerReferences(es *esv1.Elasticsearch) []metav1.OwnerReference {
	return []metav1.OwnerReference{{
		APIVersion:         "elasticsearch.k8s.elastic.co/v1",
		Kind:               "Elasticsearch",
		Name:               es.Name,
		UID:                es.UID,
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}}
}

func getServiceName(st serviceType, clusterName string) string {
	switch st {
	case transport:
		return services.TransportServiceName(clusterName)
	case external:
		return services.ExternalServiceName(clusterName)
	case internal:
		return services.InternalServiceName(clusterName)
	case remote:
		return services.RemoteClusterServiceName(clusterName)
	}
	return ""
}

func getPortForService(st serviceType) corev1.ServicePort {
	switch st {
	case transport:
		return corev1.ServicePort{
			Name:     "tls-transport",
			Protocol: corev1.ProtocolTCP,
			Port:     network.TransportPort,
		}
	case external:
		return corev1.ServicePort{
			Name: "http",
			Port: network.HTTPPort,
		}
	case internal:
		return corev1.ServicePort{
			Name:     "https",
			Protocol: corev1.ProtocolTCP,
			Port:     network.HTTPPort,
		}
	case remote:
		return corev1.ServicePort{
			Name:     "rcs",
			Protocol: corev1.ProtocolTCP,
			Port:     network.RemoteClusterPort,
		}
	}
	return corev1.ServicePort{}
}

const staticPassword = "password"

type staticPasswordGenerator struct{}

func (s *staticPasswordGenerator) Generate(_ context.Context) ([]byte, error) {
	return []byte(staticPassword), nil
}

func mustGetParams(t *testing.T, esServer *httptest.Server, numRemoteCAs int, initK8sObjects ...client.Object) driver.Parameters {
	watches := watches2.NewDynamicWatches()
	passwordHasher, err := cryptutil.NewPasswordHasher(8)
	require.NoError(t, err)

	operatorParams := operator.Parameters{
		PasswordGenerator: &staticPasswordGenerator{},
		PasswordHasher:    passwordHasher,
		GlobalCA:          nil,
		CACertRotation: certificates.RotationParams{
			Validity:     120 * time.Hour,
			RotateBefore: 1 * time.Hour,
		},
		CertRotation: certificates.RotationParams{
			Validity:     120 * time.Hour,
			RotateBefore: 1 * time.Hour,
		},
	}

	state, err := reconcile.NewState(standardElasticsearch)
	require.NoError(t, err)

	urlProvider := client2.NewStaticURLProvider(esServer.URL)

	baselineObjects := append(generateRemoteCAs(standardElasticsearch.Namespace, standardElasticsearch.Name, numRemoteCAs), getServiceAccountTokenSecret(&standardElasticsearch))
	initK8sObjects = append(initK8sObjects, baselineObjects...)

	return driver.Parameters{
		Client:             k8s.NewFakeClient(initK8sObjects...),
		ES:                 standardElasticsearch,
		ReconcileState:     state,
		DynamicWatches:     watches,
		OperatorParameters: operatorParams,
		URLProvider:        urlProvider,
		Observers:          observer.NewManager(10*time.Nanosecond, nil),
		SupportedVersions:  *version.SupportedVersions(mustParseVersion(t, "9.3.1")),
	}
}

func mustParseVersion(t *testing.T, version string) commonversion.Version {
	v, err := commonversion.Parse(version)
	require.NoError(t, err)

	return v
}
