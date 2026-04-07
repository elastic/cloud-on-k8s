// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build integration

package shared

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"fmt"
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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
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
		HTTP: commonv1.HTTPConfigWithClientOptions{
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
		expectedCerts        map[string]expectedCertData
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
			expectedCerts:      buildExpectedCertData(baseStatefulElasticsearch, 0),
			expectedSecrets:    mustBuildExpectedSecrets(t, &baseStatefulElasticsearch, "1"),
			expectedConfigMaps: mustBuildExpectedConfigMaps(t, &baseStatefulElasticsearch, "1", esServer),
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
			expectedCerts:      buildExpectedCertData(baseStatefulElasticsearch, 1),
			expectedSecrets:    mustBuildExpectedSecrets(t, &baseStatefulElasticsearch, "1"),
			expectedConfigMaps: mustBuildExpectedConfigMaps(t, &baseStatefulElasticsearch, "1", esServer),
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
			name: "failure generating remote-ca secret does not block remaining reconciliation",
			params: func() driver.Parameters {
				p := mustBuildParams(t, esServer, baseStatefulElasticsearch.Spec.Version, services.InternalServiceURL(baseStatefulElasticsearch), 1,
					&baseStatefulElasticsearch,
					mustBuildNewPod(t, &baseStatefulElasticsearch, esServer.Listener.Addr(), baseStatefulElasticsearch.Labels[label.VersionLabelName]),
				)
				es := baseStatefulElasticsearch.DeepCopy()
				es.Spec.RemoteClusterServer.Enabled = true
				p.ES = *es

				remoteCASecretKey := types.NamespacedName{Namespace: es.Namespace, Name: esv1.RemoteCaSecretName(es.Name)}
				p.Client = &fakeClient{
					Client: p.Client,
					errors: map[client.ObjectKey]error{remoteCASecretKey: errors.New("beep boop I'm an error")},
				}

				return p
			}(),
			expectedServices: map[string]corev1.Service{
				esv1.TransportService(clusterName):             newService(&baseStatefulElasticsearch, transport, "1"),
				services.ExternalServiceName(clusterName):      newService(&baseStatefulElasticsearch, external, "1"),
				services.InternalServiceName(clusterName):      newService(&baseStatefulElasticsearch, internal, "1"),
				services.RemoteClusterServiceName(clusterName): newService(&baseStatefulElasticsearch, remote, "1"),
			},
			expectedCerts: func() map[string]expectedCertData {
				baseCertData := buildExpectedCertData(baseStatefulElasticsearch, 1)
				delete(baseCertData, esv1.RemoteCaSecretName(baseStatefulElasticsearch.Name))
				return baseCertData
			}(),
			expectedSecrets:    mustBuildExpectedSecrets(t, &baseStatefulElasticsearch, "1"),
			expectedConfigMaps: mustBuildExpectedConfigMaps(t, &baseStatefulElasticsearch, "1", esServer),
			expectedState: &ReconcileState{
				Meta: metadata.Metadata{
					Labels: map[string]string{
						label.ClusterNameLabelName:   clusterName,
						"common.k8s.elastic.co/type": "elasticsearch",
					},
					Annotations: nil,
				},
				ESReachable: true, // still reachable despite remoteca secret creation failure
			},
			expectReconciliation: false, // false because results.incomplete is true due to the remoteca secret creation failure
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
			expectedCerts:      buildExpectedCertData(baseStatefulElasticsearch, 0),
			expectedSecrets:    mustBuildExpectedSecrets(t, &baseStatefulElasticsearch, "1"),
			expectedConfigMaps: mustBuildExpectedConfigMaps(t, &baseStatefulElasticsearch, "1", esServer),
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
			expectedCerts:      buildExpectedCertData(baseStatefulElasticsearch, 0),
			expectedSecrets:    mustBuildExpectedSecrets(t, &baseStatefulElasticsearch, "1"),
			expectedConfigMaps: mustBuildExpectedConfigMaps(t, &baseStatefulElasticsearch, "1", nil),
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
			expectedCerts:      buildExpectedCertData(baseStatefulElasticsearch, 0),
			expectedSecrets:    mustBuildExpectedSecrets(t, &baseStatefulElasticsearch, "1"),
			expectedConfigMaps: mustBuildExpectedConfigMaps(t, &baseStatefulElasticsearch, "1", esServer),
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
			expectedCerts: buildExpectedCertData(baseStatefulElasticsearch, 0),
			expectedSecrets: func() map[string]corev1.Secret {
				// fails before file settings secret is created
				baseSecrets := mustBuildExpectedSecrets(t, &baseStatefulElasticsearch, "1")
				delete(baseSecrets, esv1.FileSettingsSecretName(baseStatefulElasticsearch.Name))
				return baseSecrets
			}(),
			expectedConfigMaps: func() map[string]corev1.ConfigMap {
				defaultConfigMaps := mustBuildExpectedConfigMaps(t, &baseStatefulElasticsearch, "1", esServer)
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

			s, results := ReconcileSharedResources(context.Background(), testDriver, tt.params, false)
			if tt.expectedState != nil {
				require.NotNil(t, s, "Expected non-nil state")
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
				assert.True(t, s.ESClient.HasProperties(expectedVersion, esclient.BasicAuth{Name: user.ControllerUserName, Password: staticPassword}, tt.params.URLProvider, expectedClientCerts, nil), "Generated Elasticsearch client does not have expected properties")
			} else {
				assert.Nil(t, s, "Expected nil state")
			}
			actualIsReconciled, _ := results.IsReconciled()
			assert.EqualValues(t, tt.expectReconciliation, actualIsReconciled, "Expected reconciliation to be %v, got %v", tt.expectReconciliation, actualIsReconciled)

			assert.Equal(t, tt.expectError, results.HasError(), "expected error on results")

			// Ensure expected secrets are created and match expected structure/content
			actualSecrets := mustGetActualSecrets(t, k8sClient, namespace)
			expectedSecrets := len(tt.expectedSecrets) + len(tt.expectedCerts)
			assert.Lenf(t, actualSecrets.Items, expectedSecrets, "Expected %d secrets but got %d", expectedSecrets, len(actualSecrets.Items))
			for _, actualSecret := range actualSecrets.Items {
				expectedSecret, expectedSecretFound := tt.expectedSecrets[actualSecret.Name]
				expectedCert, expectedCertFound := tt.expectedCerts[actualSecret.Name]
				require.Falsef(t, expectedSecretFound && expectedCertFound, "Secret [%s] should only be defined once in the expectations", actualSecret.Name)

				if expectedSecretFound {
					assertSecretEquality(t, expectedSecret, actualSecret)
				}

				if expectedCertFound {
					assertCertificatesEqual(t, expectedCert, actualSecret)
				}
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

func buildExpectedCertData(es esv1.Elasticsearch, numRemoteCAs int) map[string]expectedCertData {
	httpCACommonName := es.Name + "-http"
	httpCommonName := es.Name + "-es-http.test-ns.es.local"
	httpCAIssuer := pkix.Name{
		OrganizationalUnit: []string{es.Name},
		CommonName:         httpCACommonName,
		Names: []pkix.AttributeTypeAndValue{
			{
				Type:  certificates.OrganizationalUnitIdentifier,
				Value: es.Name,
			},
			{
				Type:  certificates.CommonNameObjectIdentifier,
				Value: httpCACommonName,
			},
		},
	}
	httpSubject := pkix.Name{
		OrganizationalUnit: []string{es.Name},
		CommonName:         httpCommonName,
		Names: []pkix.AttributeTypeAndValue{
			{
				Type:  certificates.OrganizationalUnitIdentifier,
				Value: es.Name,
			},
			{
				Type:  certificates.CommonNameObjectIdentifier,
				Value: httpCommonName,
			},
		},
	}

	transportCACommonName := es.Name + "-transport"
	transportIssuer := pkix.Name{
		OrganizationalUnit: []string{es.Name},
		CommonName:         transportCACommonName,
		Names: []pkix.AttributeTypeAndValue{
			{
				Type:  certificates.OrganizationalUnitIdentifier,
				Value: es.Name,
			},
			{
				Type:  certificates.CommonNameObjectIdentifier,
				Value: transportCACommonName,
			},
		},
	}
	cd := map[string]expectedCertData{
		// http ca internal
		certificates.CAInternalSecretName(esv1.ESNamer, es.Name, certificates.HTTPCAType): {
			certificates.CertFileName: {
				{
					issuer:   httpCAIssuer,
					subject:  httpCAIssuer,
					isCA:     true,
					dnsNames: nil,
				},
			},
			certificates.KeyFileName: {},
		},
		// http certs internal
		certificates.InternalCertsSecretName(esv1.ESNamer, es.Name): {
			certificates.CertFileName: {
				{
					issuer:  httpCAIssuer,
					subject: httpSubject,
					isCA:    false,
					dnsNames: []string{
						httpCommonName,
						es.Name + "-es-http",
						es.Name + "-es-http.test-ns.svc",
						es.Name + "-es-http.test-ns",
						es.Name + "-es-internal-http.test-ns.svc",
						es.Name + "-es-internal-http.test-ns",
					},
				},
				{
					issuer:   httpCAIssuer,
					subject:  httpCAIssuer,
					isCA:     true,
					dnsNames: nil,
				},
			},
			certificates.KeyFileName: {},
			certificates.CAFileName: {
				{
					issuer:  httpCAIssuer,
					subject: httpCAIssuer,
					isCA:    true,
				},
			},
		},
		// http certs public
		certificates.PublicCertsSecretName(esv1.ESNamer, es.Name): {
			certificates.CertFileName: {
				{
					issuer:  httpCAIssuer,
					subject: httpSubject,
					isCA:    false,
					dnsNames: []string{
						httpCommonName,
						es.Name + "-es-http",
						es.Name + "-es-http.test-ns.svc",
						es.Name + "-es-http.test-ns",
						es.Name + "-es-internal-http.test-ns.svc",
						es.Name + "-es-internal-http.test-ns",
					},
				},
				{
					issuer:   httpCAIssuer,
					subject:  httpCAIssuer,
					isCA:     true,
					dnsNames: nil,
				},
			},
			certificates.CAFileName: {
				{
					issuer:   httpCAIssuer,
					subject:  httpCAIssuer,
					isCA:     true,
					dnsNames: nil,
				},
			},
		},
		// transport ca internal
		certificates.CAInternalSecretName(esv1.ESNamer, es.Name, certificates.TransportCAType): {
			certificates.CertFileName: {
				{
					issuer:   transportIssuer,
					subject:  transportIssuer,
					isCA:     true,
					dnsNames: nil,
				},
			},
			certificates.KeyFileName: {},
		},
		// transport certs public
		certificates.PublicTransportCertsSecretName(esv1.ESNamer, es.Name): {
			certificates.CAFileName: {
				{
					issuer:   transportIssuer,
					subject:  transportIssuer,
					isCA:     true,
					dnsNames: nil,
				},
			},
		},
		// remote ca
		esv1.RemoteCaSecretName(es.Name): {
			certificates.CAFileName: {
				{
					issuer:   transportIssuer,
					subject:  transportIssuer,
					isCA:     true,
					dnsNames: nil,
				},
			},
		},
	}

	if numRemoteCAs > 0 {
		data := make([]certData, 0, numRemoteCAs)
		for i := range numRemoteCAs {
			commonName := fmt.Sprintf("%s-remote-ca-%d", es.Name, i)
			remoteCAIssuer := pkix.Name{
				CommonName:         commonName,
				OrganizationalUnit: []string{fmt.Sprintf("%s-%d", es.Name, i)},
				Names: []pkix.AttributeTypeAndValue{
					{
						Type:  certificates.OrganizationalUnitIdentifier,
						Value: fmt.Sprintf("%s-%d", es.Name, i),
					},
					{
						Type:  certificates.CommonNameObjectIdentifier,
						Value: commonName,
					},
				},
			}
			data = append(data, certData{
				issuer:   remoteCAIssuer,
				subject:  remoteCAIssuer,
				isCA:     true,
				dnsNames: nil,
			})
		}
		cd[esv1.RemoteCaSecretName(es.Name)] = expectedCertData{
			certificates.CAFileName: data,
		}
	}

	return cd
}

// mustGetActualSecrets gets all actual secrets created by ReconcileSharedResources, excluding the test-generated
// secrets with the label common.k8s.elastic.co/type=remote-ca
func mustGetActualSecrets(t *testing.T, k8sClient k8s.Client, namespace string) *corev1.SecretList {
	t.Helper()
	actualSecrets := &corev1.SecretList{}
	requirement, err := labels.NewRequirement(commonv1.TypeLabelName, selection.NotIn, []string{"remote-ca"})
	require.NoErrorf(t, err, "Failed to create requirement for type label: %v", err)
	selector := labels.NewSelector().Add(*requirement)
	require.NoError(t, k8sClient.List(context.Background(), actualSecrets, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: selector}))
	return actualSecrets
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

func assertCertificatesEqual(t *testing.T, expected expectedCertData, actual corev1.Secret) {
	assert.Lenf(t, actual.Data, len(expected), "secret [%s] Data has unexpected number of items", actual.Name)
	for name, cert := range actual.Data {
		expectedCerts, ok := expected[name]
		require.Truef(t, ok, "actual has Data key %q missing from expected secret %s", name, actual.Name)
		if name == certificates.KeyFileName {
			key, err := certificates.ParsePEMPrivateKey(cert)
			assert.NoErrorf(t, err, "error parsing secret %s %s key", actual.Name, name)
			rsaKey, ok := key.(*rsa.PrivateKey)
			assert.Truef(t, ok, "secret %s %s key is not an RSA private key", actual.Name, name)
			assert.NoErrorf(t, rsaKey.Validate(), "secret %s %s key is invalid", actual.Name, name)
		} else {
			actualCerts, err := certificates.ParsePEMCerts(cert)
			assert.NoErrorf(t, err, "error parsing secret %s %s cert", actual.Name, name)
			require.Greaterf(t, len(actualCerts), 0, "secret %s %s cert should have at least 1 PEM cert", actual.Name, name)
			require.Lenf(t, actualCerts, len(expectedCerts), "secret %s %s cert expected %d PEM certs", actual.Name, name, len(expectedCerts))
			for i, actualCert := range actualCerts {
				expectedCert := expectedCerts[i]
				assert.Truef(t, actualCert.NotAfter.After(time.Now()), "secret %s %s %d cert is expired", actual.Name, name, i)
				assert.Truef(t, actualCert.NotBefore.Before(time.Now()), "secret %s %s %d cert NotBefore set incorrectly", actual.Name, name, i)
				assert.Equalf(t, expectedCert.isCA, actualCert.IsCA, "secret %s %s %d cert IsCA set incorrectly", actual.Name, name, i)
				assert.Equalf(t, expectedCert.dnsNames, actualCert.DNSNames, "secret %s %s %d cert DNSNames set incorrectly", actual.Name, name, i)
				assert.Equalf(t, expectedCert.issuer, actualCert.Issuer, "secret %s %s %d cert Issuer set incorrectly", actual.Name, name, i)
				assert.Equalf(t, expectedCert.subject, actualCert.Subject, "secret %s %s %d cert Subject set incorrectly", actual.Name, name, i)
			}
		}
	}
}

// assertSecretEquality verifies that actual non-certificate secrets match expected non-certificate secrets.
// Certificate secrets are returned for later validation.
func assertSecretEquality(t *testing.T, expected, actual corev1.Secret) {
	assert.Equalf(t, expected.ObjectMeta.Name, actual.ObjectMeta.Name, "secret %s has incorrect object metadata", actual.Name)
	assert.Equalf(t, expected.Type, actual.Type, "secret %s has incorrect type", actual.Name)
	if strings.Contains(actual.Name, "file-settings") {
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

func mustBuildExpectedConfigMaps(t *testing.T, es *esv1.Elasticsearch, resourceVersion string, esServer *httptest.Server) map[string]corev1.ConfigMap {
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

// mustBuildExpectedSecrets builds the map of non-certificate secrets we expect in the namespace after ReconcileSharedResources.
// User/role material uses fixed fixtures
func mustBuildExpectedSecrets(t *testing.T, es *esv1.Elasticsearch, resourceVersion string) map[string]corev1.Secret {
	t.Helper()
	labels := label.NewLabels(types.NamespacedName{
		Namespace: es.Namespace,
		Name:      es.Name,
	})
	labels["eck.k8s.elastic.co/credentials"] = "true"
	labels["eck.k8s.elastic.co/owner-kind"] = ""
	labels["eck.k8s.elastic.co/owner-name"] = es.Name
	labels["eck.k8s.elastic.co/owner-namespace"] = es.Namespace

	// Non-certificate secrets (users, roles, service account token)
	serviceAccountTokenSecret := newServiceAccountTokenSecret(es)
	fileSettings := filesettings.NewEmptySettings(time.Now().Unix())
	fileSettingsData, err := json.Marshal(fileSettings)
	require.NoError(t, err, "error marshalling file-settings")

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

	return result
}

func mustGetPredefinedRoles(t *testing.T) []byte {
	t.Helper()
	predefinedRoles, err := user.PredefinedRoles.FileBytes()
	require.NoError(t, err, "error reading predefined roles")
	return predefinedRoles
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
				OrganizationalUnit: []string{fmt.Sprintf("%s-%d", name, i)},
			},
			ExpireIn: &validity,
		})
		require.NoError(t, err, "error generating remote CA")
		caCertPEM := certificates.EncodePEMCert(ca.Cert.Raw)

		secretsToCreate[i] = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("remote-ca-%s-%d", name, i),
				Namespace: namespace,
				Labels: map[string]string{
					label.ClusterNameLabelName: name,
					commonv1.TypeLabelName:     "remote-ca",
				},
			},
			Data: map[string][]byte{
				certificates.CAFileName: caCertPEM,
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
	t.Helper()
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

type expectedCertData map[string][]certData

type certData struct {
	issuer   pkix.Name
	subject  pkix.Name
	isCA     bool
	dnsNames []string
}

const staticPassword = "password"

type staticPasswordGenerator struct{}

func (s *staticPasswordGenerator) Generate(_ context.Context) ([]byte, error) {
	return []byte(staticPassword), nil
}

// Length returns the fixed password length used by staticPasswordGenerator.
func (s *staticPasswordGenerator) Length(_ context.Context) int {
	return len(staticPassword)
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

	// Deep-copy to avoid cross-test contamination of the shared baseStatefulElasticsearch annotations map.
	es := *baseStatefulElasticsearch.DeepCopy()
	state, err := reconcile.NewState(es)
	require.NoError(t, err)

	// Because we don't have a clean way of mocking the kubernetes DNS resolution, Elasticsearch will never be reachable
	// unless we set the URL to the esServer URL
	urlProvider := esclient.NewStaticURLProvider(defaultURL)
	if listeningServer != nil {
		urlProvider = esclient.NewStaticURLProvider(listeningServer.URL) // required to get a response from the mock server
	}

	remoteCAs := mustGenerateRemoteCASecrets(t, baseStatefulElasticsearch.Namespace, baseStatefulElasticsearch.Name, numRemoteCAs)
	serviceAccountSecret := newServiceAccountTokenSecret(&baseStatefulElasticsearch)

	baselineObjects := append(remoteCAs, serviceAccountSecret)
	initK8sObjects = append(initK8sObjects, baselineObjects...)

	k8sClient := k8s.NewFakeClient(initK8sObjects...)
	return driver.Parameters{
		Client:             k8sClient,
		ES:                 es,
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

type fakeClient struct {
	k8s.Client
	errors map[client.ObjectKey]error
}

func (f *fakeClient) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	if err := f.errors[key]; err != nil {
		return err
	}
	return f.Client.Get(context.Background(), key, obj)
}

func (f *fakeClient) Create(_ context.Context, obj client.Object, _ ...client.CreateOption) error {
	key := client.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()}
	if err := f.errors[key]; err != nil {
		return err
	}
	return f.Client.Create(context.Background(), obj)
}

var _ k8s.Client = (*fakeClient)(nil)

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
