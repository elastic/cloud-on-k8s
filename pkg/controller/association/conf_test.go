// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func TestFetchWithAssociation(t *testing.T) {
	t.Run("apmServer", testFetchAPMServer)
	t.Run("kibana", testFetchKibana)
}

func testFetchAPMServer(t *testing.T) {
	testCases := []struct {
		name                string
		apmServer           *apmv1.ApmServer
		request             reconcile.Request
		wantErr             bool
		wantEsAssocConf     *commonv1.AssociationConf
		wantKibanaAssocConf *commonv1.AssociationConf
	}{
		{
			name:      "with es association annotation",
			apmServer: newTestAPMServer().withEsConfAnnotations().build(),
			request:   reconcile.Request{NamespacedName: types.NamespacedName{Name: "apm-server-test", Namespace: "apm-ns"}},
			wantEsAssocConf: &commonv1.AssociationConf{
				AuthSecretName: "auth-secret",
				AuthSecretKey:  "apm-user",
				CASecretName:   "ca-secret",
				URL:            "https://es.svc:9300",
			},
		},
		{
			name:      "with es and kibana association annotations",
			apmServer: newTestAPMServer().withEsConfAnnotations().withKbConfAnnotations().build(),
			request:   reconcile.Request{NamespacedName: types.NamespacedName{Name: "apm-server-test", Namespace: "apm-ns"}},
			wantEsAssocConf: &commonv1.AssociationConf{
				AuthSecretName: "auth-secret",
				AuthSecretKey:  "apm-user",
				CASecretName:   "ca-secret",
				URL:            "https://es.svc:9300",
			},
			wantKibanaAssocConf: &commonv1.AssociationConf{
				AuthSecretName: "auth-secret",
				AuthSecretKey:  "apm-kb-user",
				CASecretName:   "ca-secret",
				URL:            "https://kb.svc:5601",
			},
		},
		{
			name:      "without association annotation",
			apmServer: newTestAPMServer().build(),
			request:   reconcile.Request{NamespacedName: types.NamespacedName{Name: "apm-server-test", Namespace: "apm-ns"}},
		},
		{
			name:      "non existent",
			apmServer: newTestAPMServer().build(),
			request:   reconcile.Request{NamespacedName: types.NamespacedName{Name: "some-other-apm", Namespace: "apm-ns"}},
			wantErr:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := k8s.WrappedFakeClient(tc.apmServer)

			var got apmv1.ApmServer
			err := FetchWithAssociations(context.Background(), client, tc.request, &got)

			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.Equal(t, "apm-server-test", got.Name)
			require.Equal(t, "apm-ns", got.Namespace)
			require.Equal(t, "test-image", got.Spec.Image)
			require.EqualValues(t, 1, got.Spec.Count)
			for _, assoc := range got.GetAssociations() {
				switch assoc.AssociatedType() {
				case "elasticsearch":
					require.Equal(t, tc.wantEsAssocConf, assoc.AssociationConf())
				case "kibana":
					require.Equal(t, tc.wantKibanaAssocConf, assoc.AssociationConf())
				default:
					t.Fatalf("unknown association type: %s", assoc.AssociatedType())
				}
			}

		})
	}
}

func testFetchKibana(t *testing.T) {
	testCases := []struct {
		name          string
		kibana        *kbv1.Kibana
		request       reconcile.Request
		wantErr       bool
		wantAssocConf *commonv1.AssociationConf
	}{
		{
			name:    "with association annotation",
			kibana:  mkKibana(true),
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "kb-test", Namespace: "kb-ns"}},
			wantAssocConf: &commonv1.AssociationConf{
				AuthSecretName: "auth-secret",
				AuthSecretKey:  "kb-user",
				CASecretName:   "ca-secret",
				URL:            "https://es.svc:9300",
			},
		},
		{
			name:    "without association annotation",
			kibana:  mkKibana(false),
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "kb-test", Namespace: "kb-ns"}},
		},
		{
			name:    "non existent",
			kibana:  mkKibana(true),
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "some-other-kb", Namespace: "kb-ns"}},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := k8s.WrappedFakeClient(tc.kibana)

			var got kbv1.Kibana
			err := FetchWithAssociations(context.Background(), client, tc.request, &got)

			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.Equal(t, "kb-test", got.Name)
			require.Equal(t, "kb-ns", got.Namespace)
			require.Equal(t, "test-image", got.Spec.Image)
			require.EqualValues(t, 1, got.Spec.Count)
			require.Equal(t, tc.wantAssocConf, got.AssociationConf())
		})
	}
}

func mkKibana(withAnnotations bool) *kbv1.Kibana {
	kb := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kb-test",
			Namespace: "kb-ns",
		},
		Spec: kbv1.KibanaSpec{
			Image: "test-image",
			Count: 1,
		},
	}

	if withAnnotations {
		kb.ObjectMeta.Annotations = map[string]string{
			kb.AssociationConfAnnotationName(): `{"authSecretName":"auth-secret", "authSecretKey":"kb-user", "caSecretName": "ca-secret", "url":"https://es.svc:9300"}`,
		}
	}

	return kb
}

func TestAreConfiguredIfSet(t *testing.T) {
	tests := []struct {
		name         string
		associations []commonv1.Association
		recorder     *record.FakeRecorder
		wantEvent    bool
		want         bool
	}{
		{
			name:         "All associations are configured",
			recorder:     record.NewFakeRecorder(100),
			associations: newTestAPMServer().withElasticsearchRef().withElasticsearchAssoc().withKibanaRef().withKibanaAssoc().build().GetAssociations(),
			wantEvent:    false,
			want:         true,
		},
		{
			name:         "One association is not configured",
			recorder:     record.NewFakeRecorder(100),
			associations: newTestAPMServer().withElasticsearchRef().withElasticsearchAssoc().withKibanaRef().build().GetAssociations(),
			wantEvent:    true,
			want:         false,
		},
		{
			name:         "All associations are not configured",
			recorder:     record.NewFakeRecorder(100),
			associations: newTestAPMServer().withElasticsearchRef().withKibanaRef().build().GetAssociations(),
			wantEvent:    true,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AreConfiguredIfSet(tt.associations, tt.recorder)
			if got != tt.want {
				t.Errorf("AreConfiguredIfSet() got = %v, want %v", got, tt.want)
			}
			event := fetchEvent(tt.recorder)
			if len(event) > 0 != tt.wantEvent {
				t.Errorf("emitted event = %v, want %v", len(event), tt.wantEvent)
			}
		})
	}
}

func TestElasticsearchAuthSettings(t *testing.T) {
	apmEsAssociation := apmv1.ApmEsAssociation{
		ApmServer: &apmv1.ApmServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "apm-server-sample",
				Namespace: "default",
			},
			Spec: apmv1.ApmServerSpec{},
		},
	}

	apmEsAssociation.SetAssociationConf(&commonv1.AssociationConf{
		URL: "https://elasticsearch-sample-es-http.default.svc:9200",
	})

	tests := []struct {
		name         string
		client       k8s.Client
		assocConf    commonv1.AssociationConf
		wantUsername string
		wantPassword string
		wantErr      bool
	}{
		{
			name: "When auth details are defined",
			client: k8s.WrappedFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apmelasticsearchassociation-sample-elastic-internal-apm",
					Namespace: "default",
				},
				Data: map[string][]byte{"elastic-internal-apm": []byte("a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy")},
			}),
			assocConf: commonv1.AssociationConf{
				AuthSecretName: "apmelasticsearchassociation-sample-elastic-internal-apm",
				AuthSecretKey:  "elastic-internal-apm",
				CASecretName:   "ca-secret",
				URL:            "https://elasticsearch-sample-es-http.default.svc:9200",
			},
			wantUsername: "elastic-internal-apm",
			wantPassword: "a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy",
		},
		{
			name: "When auth details are undefined",
			client: k8s.WrappedFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apmelasticsearchassociation-sample-elastic-internal-apm",
					Namespace: "default",
				},
				Data: map[string][]byte{"elastic-internal-apm": []byte("a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy")},
			}),
			assocConf: commonv1.AssociationConf{
				CASecretName: "ca-secret",
				URL:          "https://elasticsearch-sample-es-http.default.svc:9200",
			},
		},
		{
			name: "When the auth secret does not exist",
			client: k8s.WrappedFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{"elastic-internal-apm": []byte("a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy")},
			}),
			assocConf: commonv1.AssociationConf{
				AuthSecretName: "apmelasticsearchassociation-sample-elastic-internal-apm",
				AuthSecretKey:  "elastic-internal-apm",
				CASecretName:   "ca-secret",
				URL:            "https://elasticsearch-sample-es-http.default.svc:9200",
			},
			wantErr: true,
		},
		{
			name: "When the auth secret key does not exist",
			client: k8s.WrappedFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apmelasticsearchassociation-sample-elastic-internal-apm",
					Namespace: "default",
				},
				Data: map[string][]byte{"elastic-internal-apm": []byte("a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy")},
			}),
			assocConf: commonv1.AssociationConf{
				AuthSecretName: "apmelasticsearchassociation-sample-elastic-internal-apm",
				AuthSecretKey:  "bad-key",
				CASecretName:   "ca-secret",
				URL:            "https://elasticsearch-sample-es-http.default.svc:9200",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apmEsAssociation.SetAssociationConf(&tt.assocConf)
			gotUsername, gotPassword, err := ElasticsearchAuthSettings(tt.client, &apmEsAssociation)
			if (err != nil) != tt.wantErr {
				t.Errorf("getCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotUsername != tt.wantUsername {
				t.Errorf("getCredentials() gotUsername = %v, want %v", gotUsername, tt.wantUsername)
			}
			if gotPassword != tt.wantPassword {
				t.Errorf("getCredentials() gotPassword = %v, want %v", gotPassword, tt.wantPassword)
			}
		})
	}
}

func TestUpdateAssociationConf(t *testing.T) {
	kb := mkKibana(true)
	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: "kb-test", Namespace: "kb-ns"}}
	client := k8s.WrappedFakeClient(kb)

	assocConf := &commonv1.AssociationConf{
		AuthSecretName: "auth-secret",
		AuthSecretKey:  "kb-user",
		CASecretName:   "ca-secret",
		URL:            "https://es.svc:9300",
	}

	// check the existing values
	var got kbv1.Kibana
	err := FetchWithAssociations(context.Background(), client, request, &got)
	require.NoError(t, err)
	require.Equal(t, "kb-test", got.Name)
	require.Equal(t, "kb-ns", got.Namespace)
	require.Equal(t, "test-image", got.Spec.Image)
	require.EqualValues(t, 1, got.Spec.Count)
	require.Equal(t, assocConf, got.AssociationConf())

	// update and check the new values
	newAssocConf := &commonv1.AssociationConf{
		AuthSecretName: "new-auth-secret",
		AuthSecretKey:  "new-kb-user",
		CASecretName:   "new-ca-secret",
		URL:            "https://new-es.svc:9300",
	}

	err = UpdateAssociationConf(client, &got, newAssocConf, "association.k8s.elastic.co/es-conf")
	require.NoError(t, err)

	err = FetchWithAssociations(context.Background(), client, request, &got)
	require.NoError(t, err)
	require.Equal(t, "kb-test", got.Name)
	require.Equal(t, "kb-ns", got.Namespace)
	require.Equal(t, "test-image", got.Spec.Image)
	require.EqualValues(t, 1, got.Spec.Count)
	require.Equal(t, newAssocConf, got.AssociationConf())
}

func TestRemoveAssociationConf(t *testing.T) {
	kb := mkKibana(true)
	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: "kb-test", Namespace: "kb-ns"}}
	client := k8s.WrappedFakeClient(kb)

	assocConf := &commonv1.AssociationConf{
		AuthSecretName: "auth-secret",
		AuthSecretKey:  "kb-user",
		CASecretName:   "ca-secret",
		URL:            "https://es.svc:9300",
	}

	// check the existing values
	var got kbv1.Kibana
	err := FetchWithAssociations(context.Background(), client, request, &got)
	require.NoError(t, err)
	require.Equal(t, "kb-test", got.Name)
	require.Equal(t, "kb-ns", got.Namespace)
	require.Equal(t, "test-image", got.Spec.Image)
	require.EqualValues(t, 1, got.Spec.Count)
	require.Equal(t, assocConf, got.AssociationConf())

	// remove and check the new values
	err = RemoveAssociationConf(client, &got, "association.k8s.elastic.co/es-conf")
	require.NoError(t, err)

	err = FetchWithAssociations(context.Background(), client, request, &got)
	require.NoError(t, err)
	require.Equal(t, "kb-test", got.Name)
	require.Equal(t, "kb-ns", got.Namespace)
	require.Equal(t, "test-image", got.Spec.Image)
	require.EqualValues(t, 1, got.Spec.Count)
	require.Nil(t, got.AssociationConf())
}
