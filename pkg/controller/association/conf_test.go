// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
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
			client := k8s.NewFakeClient(tc.apmServer)

			var got apmv1.ApmServer
			err := client.Get(context.Background(), tc.request.NamespacedName, &got)

			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.Equal(t, "apm-server-test", got.Name)
			require.Equal(t, "apm-ns", got.Namespace)
			require.Equal(t, "test-image", got.Spec.Image)
			require.EqualValues(t, 1, got.Spec.Count)
			for _, assoc := range got.GetAssociations() {
				assocConf, err := assoc.AssociationConf()
				require.NoError(t, err)
				switch assoc.AssociationType() {
				case "elasticsearch":
					require.Equal(t, tc.wantEsAssocConf, assocConf)
				case "kibana":
					require.Equal(t, tc.wantKibanaAssocConf, assocConf)
				default:
					t.Fatalf("unknown association type: %s", assoc.AssociationType())
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
			client := k8s.NewFakeClient(tc.kibana)

			var got kbv1.Kibana
			err := client.Get(context.Background(), tc.request.NamespacedName, &got)

			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.Equal(t, "kb-test", got.Name)
			require.Equal(t, "kb-ns", got.Namespace)
			require.Equal(t, "test-image", got.Spec.Image)
			require.EqualValues(t, 1, got.Spec.Count)
			assocConf, err := got.EsAssociation().AssociationConf()
			require.NoError(t, err)
			require.Equal(t, tc.wantAssocConf, assocConf)
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
			kb.EsAssociation().AssociationConfAnnotationName(): `{"authSecretName":"auth-secret", "authSecretKey":"kb-user", "caSecretName": "ca-secret", "url":"https://es.svc:9300"}`,
		}
		kb.Spec.ElasticsearchRef = commonv1.ObjectSelector{
			Name:      "es-test",
			Namespace: "es-ns",
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
			got, err := AreConfiguredIfSet(tt.associations, tt.recorder)
			require.NoError(t, err)
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
			client: k8s.NewFakeClient(&corev1.Secret{
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
			client: k8s.NewFakeClient(&corev1.Secret{
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
			client: k8s.NewFakeClient(&corev1.Secret{
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
			client: k8s.NewFakeClient(&corev1.Secret{
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
			gotCredentials, err := ElasticsearchAuthSettings(tt.client, &apmEsAssociation)
			if (err != nil) != tt.wantErr {
				t.Errorf("getCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotCredentials.Username != tt.wantUsername {
				t.Errorf("getCredentials() gotUsername = %v, want %v", gotCredentials.Username, tt.wantUsername)
			}
			if gotCredentials.Password != tt.wantPassword {
				t.Errorf("getCredentials() gotPassword = %v, want %v", gotCredentials.Password, tt.wantPassword)
			}
		})
	}
}

func TestUpdateAssociationConf(t *testing.T) {
	kb := mkKibana(true)
	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: "kb-test", Namespace: "kb-ns"}}
	client := k8s.NewFakeClient(kb)

	expectedAssocConf := &commonv1.AssociationConf{
		AuthSecretName: "auth-secret",
		AuthSecretKey:  "kb-user",
		CASecretName:   "ca-secret",
		URL:            "https://es.svc:9300",
	}

	// check the existing values
	var got kbv1.Kibana
	err := client.Get(context.Background(), request.NamespacedName, &got)
	require.NoError(t, err)
	require.Equal(t, "kb-test", got.Name)
	require.Equal(t, "kb-ns", got.Namespace)
	require.Equal(t, "test-image", got.Spec.Image)
	require.EqualValues(t, 1, got.Spec.Count)
	assocConf, err := got.EsAssociation().AssociationConf()
	require.NoError(t, err)
	require.Equal(t, expectedAssocConf, assocConf)

	// update and check the new values
	newAssocConf := &commonv1.AssociationConf{
		AuthSecretName: "new-auth-secret",
		AuthSecretKey:  "new-kb-user",
		CASecretName:   "new-ca-secret",
		URL:            "https://new-es.svc:9300",
	}

	err = UpdateAssociationConf(client, got.EsAssociation(), newAssocConf)
	require.NoError(t, err)

	err = client.Get(context.Background(), request.NamespacedName, &got)
	require.NoError(t, err)
	require.Equal(t, "kb-test", got.Name)
	require.Equal(t, "kb-ns", got.Namespace)
	require.Equal(t, "test-image", got.Spec.Image)
	require.EqualValues(t, 1, got.Spec.Count)
	assocConf, err = got.EsAssociation().AssociationConf()
	require.NoError(t, err)
	require.Equal(t, newAssocConf, assocConf)
}

func TestRemoveAssociationConf(t *testing.T) {
	kb := mkKibana(true)
	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: "kb-test", Namespace: "kb-ns"}}
	client := k8s.NewFakeClient(kb)

	expectedAssocConf := &commonv1.AssociationConf{
		AuthSecretName: "auth-secret",
		AuthSecretKey:  "kb-user",
		CASecretName:   "ca-secret",
		URL:            "https://es.svc:9300",
	}

	// check the existing values
	var got kbv1.Kibana
	err := client.Get(context.Background(), request.NamespacedName, &got)
	require.NoError(t, err)
	require.Equal(t, "kb-test", got.Name)
	require.Equal(t, "kb-ns", got.Namespace)
	require.Equal(t, "test-image", got.Spec.Image)
	require.EqualValues(t, 1, got.Spec.Count)
	assocConf, err := got.EsAssociation().AssociationConf()
	require.NoError(t, err)
	require.Equal(t, expectedAssocConf, assocConf)

	// remove and check the new values
	err = RemoveAssociationConf(client, got.EsAssociation())
	require.NoError(t, err)

	err = client.Get(context.Background(), request.NamespacedName, &got)
	require.NoError(t, err)
	require.Equal(t, "kb-test", got.Name)
	require.Equal(t, "kb-ns", got.Namespace)
	require.Equal(t, "test-image", got.Spec.Image)
	require.EqualValues(t, 1, got.Spec.Count)
	assocConf, err = got.EsAssociation().AssociationConf()
	require.NoError(t, err)
	require.Nil(t, assocConf)
}

func TestAllowVersion(t *testing.T) {
	apmNoAssoc := &apmv1.ApmServer{}
	apmTwoAssoc := &apmv1.ApmServer{Spec: apmv1.ApmServerSpec{
		ElasticsearchRef: commonv1.ObjectSelector{Name: "some-es"}, KibanaRef: commonv1.ObjectSelector{Name: "some-kb"}}}
	apmTwoAssocWithVersions := func(versions []string) *apmv1.ApmServer {
		apm := apmTwoAssoc.DeepCopy()
		for i, assoc := range apm.GetAssociations() {
			assoc.SetAssociationConf(&commonv1.AssociationConf{Version: versions[i]})
		}
		return apm
	}
	type args struct {
		resourceVersion version.Version
		associated      commonv1.Associated
	}
	tests := []struct {
		name      string
		args      args
		want      bool
		wantEvent bool
	}{
		{
			name: "no association specified: allow",
			args: args{
				resourceVersion: version.MustParse("7.7.0"),
				associated:      apmNoAssoc.DeepCopy(),
			},
			want: true,
		},
		{
			name: "referenced resources run the same version: allow",
			args: args{
				resourceVersion: version.MustParse("7.7.0"),
				associated:      apmTwoAssocWithVersions([]string{"7.7.0", "7.7.0"}),
			},
			want: true,
		},
		{
			name: "some referenced resources run a higher version: allow",
			args: args{
				resourceVersion: version.MustParse("7.7.0"),
				associated:      apmTwoAssocWithVersions([]string{"7.8.0", "7.7.0"}),
			},
			want: true,
		},
		{
			name: "one referenced resource runs a lower version: don't allow and emit an event",
			args: args{
				resourceVersion: version.MustParse("7.7.0"),
				associated:      apmTwoAssocWithVersions([]string{"7.7.0", "7.6.0"}),
			},
			want:      false,
			wantEvent: true,
		},
		{
			name: "no version set in the association conf: don't allow",
			args: args{
				resourceVersion: version.MustParse("7.7.0"),
				associated:      apmTwoAssocWithVersions([]string{"", ""}),
			},
			want: false,
		},
		{
			name: "association conf annotation is not set yet: don't allow",
			args: args{
				resourceVersion: version.MustParse("7.7.0"),
				associated:      apmTwoAssoc,
			},
			want: false,
		},
		{
			name: "invalid version in the association conf: don't allow",
			args: args{
				resourceVersion: version.MustParse("7.7.0"),
				associated:      apmTwoAssocWithVersions([]string{"7.7.0", "invalid"}),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		logger := log.WithValues("a", "b")
		recorder := record.NewFakeRecorder(10)
		t.Run(tt.name, func(t *testing.T) {
			if got, err := AllowVersion(tt.args.resourceVersion, tt.args.associated, logger, recorder); err != nil && got != tt.want {
				t.Errorf("AllowVersion() = %v, want %v", got, tt.want)
			}
		})
		if tt.wantEvent {
			require.NotEmpty(t, <-recorder.Events)
		} else {
			// no event expected
			select {
			case e := <-recorder.Events:
				require.Fail(t, "no event expected but got one", "event", e)
			default:
				// ok
			}
		}
	}
}

func TestRemoveObsoleteAssociationConfs(t *testing.T) {
	withAnnotations := func(annotationNames ...string) *agentv1alpha1.Agent {
		result := &agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "agent1",
				Namespace:   "namespace1",
				Annotations: make(map[string]string),
			},
		}
		for _, annotationName := range annotationNames {
			result.Annotations[annotationName] = annotationName
		}
		return result
	}

	withRefs := func(agent *agentv1alpha1.Agent, nsNames ...types.NamespacedName) *agentv1alpha1.Agent {
		for i, nsName := range nsNames {
			outputName := strconv.Itoa(i)
			if i == 0 {
				outputName = "default"
			}
			agent.Spec.ElasticsearchRefs = append(agent.Spec.ElasticsearchRefs, agentv1alpha1.Output{
				ObjectSelector: commonv1.ObjectSelector{Name: nsName.Name, Namespace: nsName.Namespace},
				OutputName:     outputName,
			})
		}
		return agent
	}

	generateAnnotationName := func(namespace, name string) string {
		agent := agentv1alpha1.Agent{
			Spec: agentv1alpha1.AgentSpec{
				ElasticsearchRefs: []agentv1alpha1.Output{{ObjectSelector: commonv1.ObjectSelector{Name: name, Namespace: namespace}}},
			},
		}
		associations := agent.GetAssociations()
		return associations[0].AssociationConfAnnotationName()
	}

	for _, tt := range []struct {
		name              string
		associated        commonv1.Associated
		wantedAnnotations []string
	}{
		{
			name:              "no annotations",
			associated:        withAnnotations(),
			wantedAnnotations: []string{},
		},
		{
			name:              "not related annotation - should be preserved",
			associated:        withAnnotations("not-related"),
			wantedAnnotations: []string{"not-related"},
		},
		{
			name: "related annotation with ref - should be preserved",
			associated: withRefs(withAnnotations(generateAnnotationName("a", "b")), types.NamespacedName{
				Namespace: "a",
				Name:      "b",
			},
			),
			wantedAnnotations: []string{generateAnnotationName("a", "b")},
		},
		{
			name:              "related annotation without ref - should be removed",
			associated:        withAnnotations(generateAnnotationName("a", "b")),
			wantedAnnotations: []string{},
		},
		{
			name: "mixed annotations - should be cleaned up",
			associated: withRefs(withAnnotations(generateAnnotationName("a", "b"), generateAnnotationName("c", "d"), "not-related"), types.NamespacedName{
				Namespace: "a",
				Name:      "b",
			},
			),
			wantedAnnotations: []string{generateAnnotationName("a", "b"), "not-related"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.NewFakeClient(tt.associated)

			require.NoError(t, RemoveObsoleteAssociationConfs(client, tt.associated, "association.k8s.elastic.co/es-conf"))

			var got agentv1alpha1.Agent
			require.NoError(t, client.Get(context.Background(), k8s.ExtractNamespacedName(tt.associated), &got))

			gotAnnotations := make([]string, 0)
			for key := range got.Annotations {
				gotAnnotations = append(gotAnnotations, key)
			}

			require.ElementsMatch(t, tt.wantedAnnotations, gotAnnotations)
		})
	}
}

func TestGetAssociationOfType(t *testing.T) {
	for _, tt := range []struct {
		name         string
		associations []commonv1.Association
		typ          commonv1.AssociationType
		wantAssoc    commonv1.Association
		wantError    bool
	}{
		{
			name:         "happy case",
			associations: []commonv1.Association{&kbv1.KibanaEntAssociation{}, &kbv1.KibanaEsAssociation{}},
			typ:          commonv1.ElasticsearchAssociationType,
			wantAssoc:    &kbv1.KibanaEsAssociation{},
			wantError:    false,
		},
		{
			name:         "no associations",
			associations: []commonv1.Association{},
			typ:          commonv1.ElasticsearchAssociationType,
			wantAssoc:    nil,
			wantError:    false,
		},
		{
			name:         "no associations found",
			associations: []commonv1.Association{&kbv1.KibanaEntAssociation{}, &kbv1.KibanaEsAssociation{}},
			typ:          commonv1.FleetServerAssociationType,
			wantAssoc:    nil,
			wantError:    false,
		},
		{
			name:         "two associations of the same type",
			associations: []commonv1.Association{&agentv1alpha1.AgentESAssociation{}, &agentv1alpha1.AgentESAssociation{}},
			typ:          commonv1.ElasticsearchAssociationType,
			wantAssoc:    nil,
			wantError:    true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotAssoc, err := SingleAssociationOfType(tt.associations, tt.typ)
			require.Equal(t, tt.wantAssoc, gotAssoc)
			require.Equal(t, tt.wantError, err != nil)
		})
	}
}
