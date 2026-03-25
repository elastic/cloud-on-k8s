// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build integration

package shared

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	toolsevents "k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	commondriver "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	commonversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	commonwatches "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user/filerealm"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

type serviceType int

const (
	transport serviceType = iota
	external
	internal
	remote
)

var baseStatefulElasticsearch = esv1.Elasticsearch{
	ObjectMeta: metav1.ObjectMeta{
		Name: "test-es", Namespace: "test-ns",
		ResourceVersion: "1",
		Annotations:     map[string]string{observer.ObserverIntervalAnnotation: "10s"},
		Labels:          map[string]string{label.VersionLabelName: "9.3.1", label.ClusterNameLabelName: "test-es"},
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: "operator.elastic.co/v1",
				Name:       "test-es-owner",
				UID:        "test-es-owner-uid",
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
		name                 string
		params               driver.Parameters
		expectedState        *ReconcileState
		expectReconciliation bool
		expectESClient       bool
		expectedServices     map[string]corev1.Service
		expectedSecrets      map[string]corev1.Secret
		expectedConfigMaps   map[string]corev1.ConfigMap
		expectError          bool
	}{
		{
			name: "happy path - new Elasticsearch with no remote cluster",
			params: mustBuildParams(t, esServer, baseStatefulElasticsearch.Spec.Version, services.InternalServiceURL(baseStatefulElasticsearch), 0,
				&baseStatefulElasticsearch,
				mustBuildNewPod(t, &baseStatefulElasticsearch, esServer.Listener.Addr(), baseStatefulElasticsearch.Labels[label.VersionLabelName]),
			),
			expectedServices: map[string]corev1.Service{
				esv1.TransportService(clusterName):        newService(&baseStatefulElasticsearch, transport, "1"),
				services.ExternalServiceName(clusterName): newService(&baseStatefulElasticsearch, external, "1"),
				services.InternalServiceName(clusterName): newService(&baseStatefulElasticsearch, internal, "1"),
			},
			expectedSecrets:    mustBuildExpectedSecrets(t, &baseStatefulElasticsearch, "1", 0),
			expectedConfigMaps: mustGetExpectedConfigMaps(t, &baseStatefulElasticsearch, "1", esServer),
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
			expectReconciliation: true,
			expectESClient:       true,
		},
		{
			name: "happy path - remote cluster service created when remote cluster server enabled",
			params: func() driver.Parameters {
				p := mustBuildParams(t, esServer, baseStatefulElasticsearch.Spec.Version, services.InternalServiceURL(baseStatefulElasticsearch), 1,
					&baseStatefulElasticsearch,
					mustBuildNewPod(t, &baseStatefulElasticsearch, esServer.Listener.Addr(), baseStatefulElasticsearch.Labels[label.VersionLabelName]),
				)
				es := baseStatefulElasticsearch.DeepCopy()
				es.Spec.RemoteClusterServer.Enabled = true
				p.ES = *es
				return p
			}(),
			expectedServices: map[string]corev1.Service{
				esv1.TransportService(clusterName):             newService(&baseStatefulElasticsearch, transport, "1"),
				services.ExternalServiceName(clusterName):      newService(&baseStatefulElasticsearch, external, "1"),
				services.InternalServiceName(clusterName):      newService(&baseStatefulElasticsearch, internal, "1"),
				services.RemoteClusterServiceName(clusterName): newService(&baseStatefulElasticsearch, remote, "1"),
			},
			expectedSecrets:    mustBuildExpectedSecrets(t, &baseStatefulElasticsearch, "1", 1),
			expectedConfigMaps: mustGetExpectedConfigMaps(t, &baseStatefulElasticsearch, "1", esServer),
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
			expectReconciliation: true,
			expectESClient:       true,
		},
		{
			name: "orphaned secret referencing a deleted pod is garbage collected",
			params: mustBuildParams(t, esServer, baseStatefulElasticsearch.Spec.Version, services.InternalServiceURL(baseStatefulElasticsearch), 0,
				&baseStatefulElasticsearch,
				mustBuildNewPod(t, &baseStatefulElasticsearch, esServer.Listener.Addr(), baseStatefulElasticsearch.Labels[label.VersionLabelName]),
				// A secret with a PodName label pointing to a pod that does not exist.
				// Zero CreationTimestamp means IsTooYoungForGC returns false, allowing deletion.
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "orphaned-pod-cert",
						Namespace: namespace,
						Labels: map[string]string{
							label.ClusterNameLabelName: clusterName,
							label.PodNameLabelName:     "deleted-pod",
						},
					},
					Data: map[string][]byte{"tls.crt": []byte("fake-cert")},
				},
			),
			expectedServices: map[string]corev1.Service{
				esv1.TransportService(clusterName):        newService(&baseStatefulElasticsearch, transport, "1"),
				services.ExternalServiceName(clusterName): newService(&baseStatefulElasticsearch, external, "1"),
				services.InternalServiceName(clusterName): newService(&baseStatefulElasticsearch, internal, "1"),
			},
			expectedSecrets:    mustBuildExpectedSecrets(t, &baseStatefulElasticsearch, "1", 0),
			expectedConfigMaps: mustGetExpectedConfigMaps(t, &baseStatefulElasticsearch, "1", esServer),
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
			expectReconciliation: true,
			expectESClient:       true,
		},
		{
			name: "failure state - no pods with master label prevents reconciliation",
			// nil listeningServer because there's no pod
			params: mustBuildParams(t, nil, baseStatefulElasticsearch.Spec.Version, services.InternalServiceURL(baseStatefulElasticsearch), 0,
				&baseStatefulElasticsearch,
			),
			expectedServices: map[string]corev1.Service{
				esv1.TransportService(clusterName):        newService(&baseStatefulElasticsearch, transport, "1"),
				services.ExternalServiceName(clusterName): newService(&baseStatefulElasticsearch, external, "1"),
				services.InternalServiceName(clusterName): newService(&baseStatefulElasticsearch, internal, "1"),
			},
			expectedSecrets:    mustBuildExpectedSecrets(t, &baseStatefulElasticsearch, "1", 0),
			expectedConfigMaps: mustGetExpectedConfigMaps(t, &baseStatefulElasticsearch, "1", nil),
			expectedState: &ReconcileState{
				Meta: metadata.Metadata{
					Labels: map[string]string{
						label.ClusterNameLabelName:   clusterName,
						"common.k8s.elastic.co/type": "elasticsearch",
					},
					Annotations: nil,
				},
				ESReachable: false,
			},
			expectReconciliation: false,
			expectESClient:       false,
		},
		{
			name: "failure state - a pod exists but is not listening",
			// nil listeningServer because the pod is not listening
			params: mustBuildParams(t, nil, baseStatefulElasticsearch.Spec.Version, services.InternalServiceURL(baseStatefulElasticsearch), 0,
				&baseStatefulElasticsearch,
				mustBuildNewPod(t, &baseStatefulElasticsearch, esServer.Listener.Addr(), baseStatefulElasticsearch.Labels[label.VersionLabelName]),
			),
			expectedServices: map[string]corev1.Service{
				esv1.TransportService(clusterName):        newService(&baseStatefulElasticsearch, transport, "1"),
				services.ExternalServiceName(clusterName): newService(&baseStatefulElasticsearch, external, "1"),
				services.InternalServiceName(clusterName): newService(&baseStatefulElasticsearch, internal, "1"),
			},
			expectedSecrets:    mustBuildExpectedSecrets(t, &baseStatefulElasticsearch, "1", 0),
			expectedConfigMaps: mustGetExpectedConfigMaps(t, &baseStatefulElasticsearch, "1", esServer),
			expectedState: &ReconcileState{
				Meta: metadata.Metadata{
					Labels: map[string]string{
						label.ClusterNameLabelName:   clusterName,
						"common.k8s.elastic.co/type": "elasticsearch",
					},
					Annotations: nil,
				},
				ESReachable: false,
			},
			expectReconciliation: false,
			expectESClient:       true,
		},
		{
			name: "failure state - a pod is running an unsupported version halts returns an error",
			// nil listeningServer because the pod is not listening
			params: mustBuildParams(t, nil, baseStatefulElasticsearch.Spec.Version, services.InternalServiceURL(baseStatefulElasticsearch), 0,
				&baseStatefulElasticsearch,
				mustBuildNewPod(t, &baseStatefulElasticsearch, esServer.Listener.Addr(), "6.0.0"),
			),
			expectedServices: map[string]corev1.Service{
				esv1.TransportService(clusterName):        newService(&baseStatefulElasticsearch, transport, "1"),
				services.ExternalServiceName(clusterName): newService(&baseStatefulElasticsearch, external, "1"),
				services.InternalServiceName(clusterName): newService(&baseStatefulElasticsearch, internal, "1"),
			},
			expectedSecrets: mustBuildExpectedSecrets(t, &baseStatefulElasticsearch, "1", 0),
			expectedConfigMaps: func() map[string]corev1.ConfigMap {
				defaultConfigMaps := mustGetExpectedConfigMaps(t, &baseStatefulElasticsearch, "1", esServer)
				delete(defaultConfigMaps, esv1.UnicastHostsConfigMap(clusterName))
				return defaultConfigMaps
			}(),
			expectedState:        nil,
			expectReconciliation: false,
			expectESClient:       false,
			expectError:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := tt.params.Client
			testDriver := commondriver.TestDriver{
				Client:       k8sClient,
				Watches:      tt.params.DynamicWatches,
				FakeRecorder: toolsevents.NewFakeRecorder(1000),
			}

			s, results := ReconcileSharedResources(context.Background(), testDriver, tt.params)
			if tt.expectedState != nil {
				assert.NotNil(t, s, "Expected non-nil state")
				assert.EqualValues(t, tt.expectedState.ESReachable, s.ESReachable)
				assert.EqualValues(t, tt.expectedState.KeystoreResources, s.KeystoreResources)
				assert.EqualValues(t, tt.expectedState.Meta, s.Meta)

				// Ensure expected ES client is created
				assert.NotNil(t, s.ESClient, "Expected non-nil ES client")
				// HasProperties inherently asserts expected certificates, user credentials, URL, and version were set
				// correctly on the client
				expectedVersion := mustParseVersion(t, tt.params.ES.Spec.Version)
				if !tt.expectESClient {
					expectedVersion = tt.params.Version
				}
				expectedClientCerts := mustGetClientCerts(t, tt.params.Client, tt.params.ES)
				assert.True(t, s.ESClient.HasProperties(expectedVersion, esclient.BasicAuth{Name: user.ControllerUserName, Password: staticPassword}, tt.params.URLProvider, expectedClientCerts), "Generated Elasticsearch client does not have expected properties")
			} else {
				assert.Nil(t, s, "Expected nil state")
			}
			actualIsReconciled, _ := results.IsReconciled()
			assert.EqualValues(t, tt.expectReconciliation, actualIsReconciled, "Expected reconciliation to be %v, got %v", tt.expectReconciliation, actualIsReconciled)

			assert.Equal(t, tt.expectError, results.HasError(), "expected error on results")

			// Ensure expected secrets are created and match expected structure/content
			actualSecrets := &corev1.SecretList{}
			assert.NoError(t, k8sClient.List(context.Background(), actualSecrets, client.InNamespace(namespace)))
			assert.Len(t, actualSecrets.Items, len(tt.expectedSecrets), "Unexpected number of secrets created")
			for _, actualSecret := range actualSecrets.Items {
				expectedSecret, ok := tt.expectedSecrets[actualSecret.Name]
				assert.Truef(t, ok, "Unexpected secret [%s] created", actualSecret.Name)
				assertSecretMatchesExpected(t, expectedSecret, actualSecret)
			}

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

// mustGetClientCerts gets the HTTP certificate secrets for the given Elasticsearch's namespace and parses
// them into a slice of x509.Certificate references.
func mustGetClientCerts(t *testing.T, client k8s.Client, es esv1.Elasticsearch) []*x509.Certificate {
	t.Helper()
	var internalCertSecret corev1.Secret
	err := client.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: certificates.InternalCertsSecretName(esv1.ESNamer, es.Name)}, &internalCertSecret)
	require.NoError(t, err, "error getting internal HTTP certificate secret")

	internalCert, err := certificates.NewCertificatesSecret(internalCertSecret)
	require.NoError(t, err, "error parsing internal HTTP certificate secret")

	clientCerts, err := certificates.ParsePEMCerts(internalCert.CertChain())
	require.NoError(t, err, "error parsing internal HTTP certificate chain")
	return clientCerts
}

// isCertOrCaSecret returns true for secrets whose Data is generated at reconciliation time
// and cannot be matched byte-for-byte, such as certificate secrets.
func isCertOrCaSecret(secretName string) bool {
	return strings.Contains(secretName, "-ca") ||
		strings.Contains(secretName, "-certs")
}

// assertSecretMatchesExpected verifies that actual secret matches expected secrets. For certificate secrets with
// generated content, data fields are checked for equality except for NotBefore and NotAfter which are time-dependent.
// NotBefore and NotAfter are only validated that time.Now is within that range; for others Data must match exactly.
func assertSecretMatchesExpected(t *testing.T, expected, actual corev1.Secret) {
	t.Helper()
	assert.Equalf(t, expected.ObjectMeta.Name, actual.ObjectMeta.Name, "secret %s has incorrect object metadata", actual.Name)
	assert.Equalf(t, expected.Type, actual.Type, "secret %s has incorrect type", actual.Name)
	if isCertOrCaSecret(expected.Name) {
		actualCertData := make([]byte, 0, 1000)
		expectedCertData := make([]byte, 0, 1000)
		assert.Lenf(t, actual.Data, len(expected.Data), "secret [%s] Data has unexpected number of items", actual.Name)
		for name, cert := range actual.Data {
			expectedCert, ok := expected.Data[name]
			require.Truef(t, ok, "actual has Data key %q missing from expected secret %s", name, expected.Name)
			// This should put the certs in the same order for comparison later
			actualCertData = append(actualCertData, cert...)
			expectedCertData = append(expectedCertData, expectedCert...)
		}
		actualCerts, err := certificates.ParsePEMCerts(actualCertData)
		assert.NoErrorf(t, err, "error parsing secret %s cert chain", actual.Name)
		expectedCerts, err := certificates.ParsePEMCerts(expectedCertData)
		require.NoError(t, err, "error parsing secret %s cert chain", expected.Name)

		for i, actualCert := range actualCerts {
			assert.Truef(t, actualCert.NotAfter.After(time.Now()), "secret %s cert is expired", actual.Name)
			assert.Truef(t, actualCert.NotBefore.Before(time.Now()), "secret %s cert NotBefore set incorrectly", actual.Name)
			expectedCert := expectedCerts[i]
			assert.Equalf(t, expectedCert.IsCA, actualCert.IsCA, "secret %s cert IsCA set incorrectly", actual.Name)
			assert.Equalf(t, expectedCert.DNSNames, actualCert.DNSNames, "secret %s cert DNSNames set incorrectly", actual.Name)
			assert.Equalf(t, expectedCert.Issuer, actualCert.Issuer, "secret %s cert Issuer set incorrectly", actual.Name)
		}
	} else if strings.Contains(actual.Name, "file-settings") {
		// The file-settings secret contains a version timestamp so equality check will always fail
		var actualSettings filesettings.Settings
		err := json.Unmarshal(actual.Data[filesettings.SettingsSecretKey], &actualSettings)
		assert.NoErrorf(t, err, "error unmarshalling actual secret %s file-settings", actual.Name)
		var expectedSettings filesettings.Settings
		err = json.Unmarshal(expected.Data[filesettings.SettingsSecretKey], &expectedSettings)
		assert.NoErrorf(t, err, "error unmarshalling expected secret %s file-settings", expected.Name)

		assert.Equalf(t, expectedSettings.State, actualSettings.State, "secret [%s] file-settings State is incorrect", actual.Name)
		assert.Equalf(t, expectedSettings.Metadata.Compatibility, actualSettings.Metadata.Compatibility, "secret [%s] file-settings Metadata is incorrect", actual.Name)

		// Approximation of version time should be within the last minute
		msInt, err := strconv.ParseInt(actualSettings.Metadata.Version, 10, 64)
		assert.NoErrorf(t, err, "error parsing actual secret %s file-settings Version into int64", actual.Name)
		actualVersion := time.UnixMilli(msInt)
		assert.Truef(t, actualVersion.After(time.Now().Add(-1*time.Minute)), "secret %s file-settings Version is too old", actual.Name)
	} else {
		assert.Equalf(t, expected.Data, actual.Data, "secret [%s] Data is incorrect", actual.Name)
	}
}

func newServiceAccountTokenSecret(es *esv1.Elasticsearch) *corev1.Secret {
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
			user.ServiceAccountTokenNameField: []byte("token-name"),
			user.ServiceAccountHashField:      []byte("hash"),
		},
	}
}

func mustGetExpectedConfigMaps(t *testing.T, es *esv1.Elasticsearch, resourceVersion string, esServer *httptest.Server) map[string]corev1.ConfigMap {
	t.Helper()
	labels := label.NewLabels(types.NamespacedName{
		Namespace: es.Namespace,
		Name:      es.Name,
	})
	ownerReferences := newOwnerReference(es)

	fsScript, err := initcontainer.RenderPrepareFsScript(es.DownwardNodeLabels())
	require.NoError(t, err, "error rendering FS script")
	preStopScript, err := nodespec.RenderPreStopHookScript(services.InternalServiceURL(*es))
	require.NoError(t, err, "error rendering preStop script")

	host := ""
	if esServer != nil {
		address := strings.Split(esServer.Listener.Addr().String(), ":") // Addr is ip:port
		host = address[0] + ":9300"
	}

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
				volume.UnicastHostsFile: host,
			},
		},
	}
}

// mustBuildExpectedSecrets builds the map of secrets we expect in the namespace after ReconcileSharedResources.
// User/role material uses fixed fixtures; certificate secrets and any other reconciler-generated secrets (keystore,
// file-settings, per-node transport certs, remote CA fixtures, etc.) are read from the client so they match the
// reconciliation output byte-for-byte.
func mustBuildExpectedSecrets(t *testing.T, es *esv1.Elasticsearch, resourceVersion string, numRemoteCAs int) map[string]corev1.Secret {
	t.Helper()
	labels := label.NewLabels(types.NamespacedName{
		Namespace: es.Namespace,
		Name:      es.Name,
	})
	labels["eck.k8s.elastic.co/credentials"] = "true"
	labels["eck.k8s.elastic.co/owner-kind"] = ""
	labels["eck.k8s.elastic.co/owner-name"] = es.Name
	labels["eck.k8s.elastic.co/owner-namespace"] = es.Namespace

	// Certificate secrets reproduced from ReconcileSharedResources (HTTP, transport, remote CA)
	certSecrets := mustBuildExpectedCertificateSecrets(t, es, labels, resourceVersion, numRemoteCAs)

	// Non-certificate secrets (users, roles, service account token)
	serviceAccountTokenSecret := newServiceAccountTokenSecret(es)
	fileSettings := filesettings.NewEmptySettings(time.Now().Unix())
	fileSettingsData, err := json.Marshal(fileSettings)
	require.NoError(t, err, "error marshalling file-settings")

	filerealm.New()
	result := map[string]corev1.Secret{
		serviceAccountTokenSecret.Name: *serviceAccountTokenSecret,
		esv1.RolesAndFileRealmSecret(es.Name): {
			ObjectMeta: metav1.ObjectMeta{
				Name:            esv1.RolesAndFileRealmSecret(es.Name),
				Namespace:       es.Namespace,
				ResourceVersion: resourceVersion,
				Labels:          labels,
			},
			Data: map[string][]byte{
				filerealm.UsersFile:        []byte(userHashes),
				filerealm.UsersRolesFile:   []byte(userRoles),
				user.RolesFile:             mustGetPredefinedRoles(t),
				user.ServiceTokensFileName: []byte("token-name:hash\n"),
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
		esv1.FileSettingsSecretName(es.Name): {
			ObjectMeta: metav1.ObjectMeta{
				Name:            esv1.FileSettingsSecretName(es.Name),
				Namespace:       es.Namespace,
				ResourceVersion: resourceVersion,
				Labels:          labels,
			},
			Data: map[string][]byte{
				filesettings.SettingsSecretKey: fileSettingsData,
			},
		},
	}
	maps.Copy(result, certSecrets)

	return result
}

func mustGetPredefinedRoles(t *testing.T) []byte {
	t.Helper()
	predefinedRoles, err := user.PredefinedRoles.FileBytes()
	require.NoError(t, err, "error reading predefined roles")
	return predefinedRoles
}

// mustBuildExpectedCertificateSecrets reproduces the certificate secrets created by ReconcileSharedResources
// (HTTP CA internal, HTTP internal/public certs, Transport CA internal, Transport public, Remote CA)
// from a given Elasticsearch object. Used to build expected secrets in tests with valid PEM data.
func mustBuildExpectedCertificateSecrets(t *testing.T, es *esv1.Elasticsearch, labels map[string]string, resourceVersion string, numRemoteCAs int) map[string]corev1.Secret {
	t.Helper()
	validity := 365 * 24 * time.Hour

	// HTTP CA (self-signed) - same as certificates.ReconcileCAAndHTTPCerts
	httpCA, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{
		Subject: pkix.Name{
			CommonName:         fmt.Sprintf("%s-%s", es.Name, certificates.HTTPCAType),
			OrganizationalUnit: []string{es.Name},
		},
		ExpireIn: &validity,
	})
	require.NoError(t, err)

	// HTTP leaf cert signed by HTTP CA - same structure as ensureInternalSelfSignedCertificateSecretContents
	httpLeafKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	require.NoError(t, err)
	commonName := es.Name + "-es-http." + es.Namespace + ".es.local"
	httpLeafTemplate := certificates.ValidatedCertificateTemplate(x509.Certificate{
		Subject: pkix.Name{
			CommonName:         commonName,
			OrganizationalUnit: []string{es.Name},
		},
		DNSNames: []string{
			commonName,
			es.Name + "-es-http",
			es.Name + "-es-http.test-ns.svc",
			es.Name + "-es-http.test-ns",
			es.Name + "-es-internal-http.test-ns.svc",
			es.Name + "-es-internal-http.test-ns",
		},
		NotBefore:          time.Now(),
		NotAfter:           time.Now().Add(validity),
		PublicKey:          httpLeafKey.Public(),
		PublicKeyAlgorithm: x509.RSA,
		SignatureAlgorithm: x509.SHA256WithRSA,
		KeyUsage:           x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:        []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	})
	httpLeafDER, err := httpCA.CreateCertificate(httpLeafTemplate)
	require.NoError(t, err)
	httpLeafPEM := certificates.EncodePEMCert(httpLeafDER, httpCA.Cert.Raw)
	httpCAPEM := certificates.EncodePEMCert(httpCA.Cert.Raw)
	httpLeafKeyPEM, err := certificates.EncodePEMPrivateKey(httpLeafKey)
	require.NoError(t, err)
	httpCAKeyPEM, err := certificates.EncodePEMPrivateKey(httpCA.PrivateKey)
	require.NoError(t, err)

	// Transport CA (self-signed) - same as transport.ReconcileOrRetrieveCA
	transportCA, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{
		Subject: pkix.Name{
			CommonName:         es.Name + "-transport",
			OrganizationalUnit: []string{es.Name},
		},
		ExpireIn: &validity,
	})
	require.NoError(t, err)
	transportCAKeyPEM, err := certificates.EncodePEMPrivateKey(transportCA.PrivateKey)
	require.NoError(t, err)
	transportCACertPEM := certificates.EncodePEMCert(transportCA.Cert.Raw)

	secrets := make(map[string]corev1.Secret)

	// HTTP CA internal
	secrets[certificates.CAInternalSecretName(esv1.ESNamer, es.Name, certificates.HTTPCAType)] = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            certificates.CAInternalSecretName(esv1.ESNamer, es.Name, certificates.HTTPCAType),
			Namespace:       es.Namespace,
			ResourceVersion: resourceVersion,
			Labels:          labels,
		},
		Data: map[string][]byte{
			certificates.CertFileName: httpCAPEM,
			certificates.KeyFileName:  httpCAKeyPEM,
		},
	}

	// HTTP internal certs (CA + leaf chain and key)
	secrets[certificates.InternalCertsSecretName(esv1.ESNamer, es.Name)] = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            certificates.InternalCertsSecretName(esv1.ESNamer, es.Name),
			Namespace:       es.Namespace,
			ResourceVersion: resourceVersion,
			Labels:          labels,
		},
		Data: map[string][]byte{
			certificates.CAFileName:   httpCAPEM,
			certificates.CertFileName: httpLeafPEM,
			certificates.KeyFileName:  httpLeafKeyPEM,
		},
	}

	// HTTP public certs (CA + leaf chain, no key)
	secrets[certificates.PublicCertsSecretName(esv1.ESNamer, es.Name)] = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            certificates.PublicCertsSecretName(esv1.ESNamer, es.Name),
			Namespace:       es.Namespace,
			ResourceVersion: resourceVersion,
			Labels:          labels,
		},
		Data: map[string][]byte{
			certificates.CAFileName:   httpCAPEM,
			certificates.CertFileName: httpLeafPEM,
		},
	}

	// Transport CA internal
	secrets[certificates.CAInternalSecretName(esv1.ESNamer, es.Name, certificates.TransportCAType)] = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            certificates.CAInternalSecretName(esv1.ESNamer, es.Name, certificates.TransportCAType),
			Namespace:       es.Namespace,
			ResourceVersion: resourceVersion,
			Labels:          labels,
		},
		Data: map[string][]byte{
			certificates.CertFileName: transportCACertPEM,
			certificates.KeyFileName:  transportCAKeyPEM,
		},
	}

	// Transport certs public (CA only)
	secrets[certificates.PublicTransportCertsSecretName(esv1.ESNamer, es.Name)] = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            certificates.PublicTransportCertsSecretName(esv1.ESNamer, es.Name),
			Namespace:       es.Namespace,
			ResourceVersion: resourceVersion,
			Labels:          labels,
		},
		Data: map[string][]byte{
			certificates.CAFileName: transportCACertPEM,
		},
	}

	var remoteCAData []byte
	if numRemoteCAs > 0 {
		remoteCAs := mustGenerateRemoteCASecrets(t, es.Namespace, es.Name, numRemoteCAs)
		allRemoteCAData := make([][]byte, 0, len(remoteCAs))
		for _, remoteCA := range remoteCAs {
			remoteCASecret, ok := remoteCA.(*corev1.Secret)
			require.True(t, ok, "remote CA [%s] is not a corev1.Secret", remoteCA.GetName())
			secrets[remoteCASecret.Name] = *remoteCASecret
			allRemoteCAData = append(allRemoteCAData, remoteCASecret.Data[certificates.CAFileName])
		}
		remoteCAData = bytes.Join(allRemoteCAData, nil)
	} else {
		// Remote CA secret - when no remote clusters, contains transport CA (remoteca.Reconcile)
		remoteCAData = transportCACertPEM
	}

	// Remote CA (concatenated remote CAs or self transport CA when none)
	remoteCAName := esv1.RemoteCaSecretName(es.Name)
	secrets[remoteCAName] = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            remoteCAName,
			Namespace:       es.Namespace,
			ResourceVersion: resourceVersion,
			Labels:          labels,
		},
		Data: map[string][]byte{
			certificates.CAFileName: remoteCAData,
		},
	}

	return secrets
}

func mustGenerateRemoteCASecrets(t *testing.T, namespace, name string, quantity int) []client.Object {
	t.Helper()
	if quantity <= 0 {
		return nil
	}
	validity := time.Hour * 24 * 365
	secretsToCreate := make([]client.Object, quantity)
	for i := range quantity {
		ca, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{
			Subject: pkix.Name{
				CommonName:         fmt.Sprintf("%s-remote-ca-%d", name, i),
				OrganizationalUnit: []string{name},
			},
			ExpireIn: &validity,
		})
		require.NoError(t, err, "error generating remote CA")

		secretsToCreate[i] = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("remote-ca-%s-%d", name, i),
				Namespace: namespace,
				Labels: map[string]string{
					label.ClusterNameLabelName:   name,
					"common.k8s.elastic.co/type": "remote-ca",
				},
			},
			Data: map[string][]byte{
				certificates.CAFileName: ca.Cert.Raw,
			},
		}
	}

	return secretsToCreate
}

func newService(es *esv1.Elasticsearch, st serviceType, resourceVersion string) corev1.Service {
	md := metadata.Metadata{
		Labels: label.NewLabels(types.NamespacedName{
			Namespace: es.Namespace,
			Name:      es.Name,
		}),
		Annotations: nil,
	}

	var service corev1.Service
	switch st {
	case transport:
		service = *services.NewTransportService(*es, md)
	case external:
		service = *services.NewExternalService(*es, md)
	case internal:
		service = *services.NewInternalService(*es, md)
	case remote:
		service = *services.NewRemoteClusterService(*es, md)
	}

	service.ObjectMeta.OwnerReferences = newOwnerReference(es)
	service.ObjectMeta.ResourceVersion = resourceVersion
	return service
}

func mustBuildNewPod(t *testing.T, es *esv1.Elasticsearch, addr net.Addr, version string) *corev1.Pod {
	ip := strings.Split(addr.String(), ":")[0]
	podName := fmt.Sprintf("%s-%s", es.Name, uuid.NewUUID()[:6])
	statefulSetName := es.Labels[label.StatefulSetNameLabelName]
	ver := mustParseVersion(t, version)
	labels := label.NewPodLabels(
		types.NamespacedName{Namespace: es.Namespace, Name: es.Name},
		statefulSetName,
		ver,
		&esv1.Node{
			Master: ptr.To[bool](true),
		},
		"https")

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

func newOwnerReference(es *esv1.Elasticsearch) []metav1.OwnerReference {
	return []metav1.OwnerReference{{
		APIVersion:         "elasticsearch.k8s.elastic.co/v1",
		Kind:               "Elasticsearch",
		Name:               es.Name,
		UID:                es.UID,
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}}
}

const staticPassword = "password"

type staticPasswordGenerator struct{}

func (s *staticPasswordGenerator) Generate(_ context.Context) ([]byte, error) {
	return []byte(staticPassword), nil
}

type staticPasswordHasher struct{}

func (s *staticPasswordHasher) ReuseOrGenerateHash(password, _ []byte) ([]byte, error) {
	return password, nil
}

func mustBuildParams(t *testing.T, listeningServer *httptest.Server, ver string, defaultURL string, numRemoteCAs int, initK8sObjects ...client.Object) driver.Parameters {
	t.Helper()
	watches := commonwatches.NewDynamicWatches()
	operatorParams := operator.Parameters{
		PasswordGenerator: &staticPasswordGenerator{},
		PasswordHasher:    &staticPasswordHasher{},
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

	state, err := reconcile.NewState(baseStatefulElasticsearch)
	require.NoError(t, err)

	// Because we don't have a clean way of mocking the kubernetes DNS resolution, Elasticsearch will never be reachable
	// unless we set the URL to the esServer URL
	urlProvider := esclient.NewStaticURLProvider(defaultURL)
	if listeningServer != nil {
		urlProvider = esclient.NewStaticURLProvider(listeningServer.URL) // required to get a response from the mock server
	}

	baselineObjects := append(mustGenerateRemoteCASecrets(t, baseStatefulElasticsearch.Namespace, baseStatefulElasticsearch.Name, numRemoteCAs), newServiceAccountTokenSecret(&baseStatefulElasticsearch))
	initK8sObjects = append(initK8sObjects, baselineObjects...)

	k8sClient := k8s.NewFakeClient(initK8sObjects...)
	return driver.Parameters{
		Client:             k8sClient,
		ES:                 baseStatefulElasticsearch,
		Version:            mustParseVersion(t, ver),
		LicenseChecker:     license.NewLicenseChecker(k8sClient, "operator-namespace"),
		ReconcileState:     state,
		DynamicWatches:     watches,
		OperatorParameters: operatorParams,
		URLProvider:        urlProvider,
		Observers:          observer.NewManager(10*time.Nanosecond, nil),
		SupportedVersions:  *version.SupportedVersions(mustParseVersion(t, "9.3.1")),
	}
}

func mustParseVersion(t *testing.T, version string) commonversion.Version {
	t.Helper()
	v, err := commonversion.Parse(version)
	require.NoError(t, err)

	return v
}

const userRoles = `elastic-internal_cluster_manage:elastic-internal-pre-stop
elastic_internal_diagnostics_v85:elastic-internal-diagnostics
elastic_internal_probe_user:elastic-internal-probe
remote_monitoring_collector:elastic-internal-monitoring
superuser:elastic,elastic-internal
`

const userHashes = `elastic-internal-diagnostics:password
elastic-internal-monitoring:password
elastic-internal-pre-stop:password
elastic-internal-probe:password
elastic-internal:password
elastic:password
`
