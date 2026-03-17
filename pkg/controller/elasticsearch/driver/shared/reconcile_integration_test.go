// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build integration

package shared

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	commondriver "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password"
	commonversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	watches2 "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	client2 "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/cryptutil"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/pointer"
)

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

func TestReconcileSharedResources(t *testing.T) {
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
	ip := strings.Split(esServer.Listener.Addr().String(), ":")[0]

	tests := []struct {
		name                   string
		params                 driver.Parameters
		expectedState          *ReconcileState
		reconciliationExpected bool
	}{
		{
			name: "happy path",
			params: mustGetParams(t, esServer, 0,
				&corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Status: corev1.ConditionTrue, Type: corev1.ContainersReady}, {Status: corev1.ConditionTrue, Type: corev1.PodReady}}}, ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "test-ns", Labels: map[string]string{label.VersionLabelName: "9.3.1", label.ClusterNameLabelName: "test-es", label.HTTPSchemeLabelName: "http", label.StatefulSetNameLabelName: esv1.StatefulSet("test-es", "sset1")}}, Spec: corev1.PodSpec{HostAliases: []corev1.HostAlias{{IP: ip, Hostnames: []string{"test-pod.test-es-es-sset1.test-ns"}}}}},
				&discoveryv1.EndpointSlice{AddressType: discoveryv1.AddressTypeIPv4, ObjectMeta: metav1.ObjectMeta{Name: "test-es-es-internal-http", Namespace: "test-ns"}, Ports: []discoveryv1.EndpointPort{{Name: ptr.To("http"), Port: pointer.Int32(9200)}}, Endpoints: []discoveryv1.Endpoint{{Hostname: ptr.To("test-pod"), Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true), Serving: ptr.To(true)}, Addresses: []string{ip}}}},
				&standardElasticsearch,
			),
			expectedState: &ReconcileState{
				Meta: metadata.Metadata{
					Labels: map[string]string{
						label.ClusterNameLabelName:   "test-es",
						"common.k8s.elastic.co/type": "elasticsearch",
					},
					Annotations: nil,
				},
				ESReachable: true,
			},
			reconciliationExpected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDriver := commondriver.TestDriver{
				Client:       tt.params.Client,
				Watches:      tt.params.DynamicWatches,
				FakeRecorder: record.NewFakeRecorder(1000),
			}

			s, results := ReconcileSharedResources(context.Background(), testDriver, tt.params)
			assert.EqualValues(t, tt.expectedState.Meta, s.Meta)
			assert.NotNil(t, s.ESClient, "Expected non-nil ES client")
			assert.EqualValues(t, tt.expectedState.ESReachable, s.ESReachable)
			assert.EqualValues(t, tt.expectedState.KeystoreResources, s.KeystoreResources)
			actualIsReconciled, _ := results.IsReconciled()
			assert.EqualValues(t, tt.reconciliationExpected, actualIsReconciled, "Expected reconciliation to be %v, got %v", tt.reconciliationExpected, actualIsReconciled)
		})
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

func mustGetParams(t *testing.T, esServer *httptest.Server, numRemoteCAs int, initK8sObjects ...client.Object) driver.Parameters {
	defer time.Sleep(2 * time.Minute)
	watches := watches2.NewDynamicWatches()
	passwordGenerator, err := password.NewRandomPasswordGenerator(8, func(_ context.Context) (bool, error) {
		return true, nil
	})
	require.NoError(t, err)
	passwordHasher, err := cryptutil.NewPasswordHasher(8)
	require.NoError(t, err)

	operatorParams := operator.Parameters{
		PasswordGenerator: passwordGenerator,
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

	initK8sObjects = append(initK8sObjects, generateRemoteCAs(standardElasticsearch.Namespace, standardElasticsearch.Name, numRemoteCAs)...)

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
