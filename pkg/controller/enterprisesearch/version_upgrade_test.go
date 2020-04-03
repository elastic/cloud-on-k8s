// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	entsname "github.com/elastic/cloud-on-k8s/pkg/controller/enterprisesearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	esUser     = "es-username"
	esPassword = "es-password"
)

var (
	esUserSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns", Name: "es-user-secret",
		},
		Data: map[string][]byte{
			esUser: []byte(esPassword),
		},
	}
	associationConf = commonv1.AssociationConf{
		URL: "es.url", CACertProvided: true, AuthSecretKey: esUser, AuthSecretName: esUserSecret.Name,
	}
)

func entSearchWithVersion(version string, annotations map[string]string) entsv1beta1.EnterpriseSearch {
	ents := entsv1beta1.EnterpriseSearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ents", Annotations: annotations},
		Spec: entsv1beta1.EnterpriseSearchSpec{Version: version}}
	ents.SetAssociationConf(&associationConf)
	return ents
}

func podWithVersion(name string, version string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns", Name: name, Labels: map[string]string{
				EnterpriseSearchNameLabelName: "ents",
				common.TypeLabelName:          Type,
				VersionLabelName:              version,
			},
		},
	}
}

func deploymentWithVersion(version string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: entsname.Deployment("ents")},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				VersionLabelName: version,
			}}}},
	}
}

// fakeRoundTrip mocks HTTP calls to the Enterprise Search API
type fakeRoundTrip struct {
	checks *roundTripChecks
}

type roundTripChecks struct {
	called           bool
	withURL          string
	withBody         string
	returnStatusCode int
}

func (f fakeRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) {
	f.checks.called = true
	f.checks.withURL = req.URL.String()
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	f.checks.withBody = string(body)
	return &http.Response{
		StatusCode: f.checks.returnStatusCode,
		Body:       ioutil.NopCloser(bytes.NewReader(nil)),
	}, nil
}

func TestVersionUpgrade_Handle(t *testing.T) {
	tests := []struct {
		name            string
		ents            entsv1beta1.EnterpriseSearch
		runtimeObjs     []runtime.Object
		httpChecks      roundTripChecks
		wantUpdatedEnts entsv1beta1.EnterpriseSearch
		wantErr         string
	}{
		{
			name: "no version upgrade: nothing to do",
			ents: entSearchWithVersion("7.7.0", nil),
			runtimeObjs: []runtime.Object{
				deploymentWithVersion("7.7.0"),
			},
			httpChecks: roundTripChecks{
				called: false,
			},
			wantUpdatedEnts: entSearchWithVersion("7.7.0", nil),
		},
		{
			name: "version upgrade requested: enable read-only mode",
			ents: entSearchWithVersion("7.7.1", nil),
			runtimeObjs: []runtime.Object{
				deploymentWithVersion("7.7.0"),
				podWithVersion("pod1", "7.7.0"),
				podWithVersion("pod2", "7.7.0"),
			},
			httpChecks: roundTripChecks{
				called:           true,
				withURL:          "https://ents-ents-http.ns.svc:3002/api/ent/v1/internal/read_only_mode",
				withBody:         "{\"enabled\": true}",
				returnStatusCode: 200,
			},
			wantUpdatedEnts: entSearchWithVersion("7.7.0", map[string]string{
				ReadOnlyModeAnnotationName: "true",
			}),
		},
		{
			name: "version upgrade requested, but no Pod running: error out",
			ents: entSearchWithVersion("7.7.1", nil),
			runtimeObjs: []runtime.Object{
				deploymentWithVersion("7.7.0"),
			},
			wantErr: "a version upgrade is scheduled, but no Pod in the prior version is running:" +
				"waiting for at least one Pod in the prior version to be running in order to enable read-only mode",
		},
		{
			name: "version upgrade requested, but annotation already set: do nothing",
			ents: entSearchWithVersion("7.7.1", map[string]string{
				ReadOnlyModeAnnotationName: "true",
			}),
			runtimeObjs: []runtime.Object{
				deploymentWithVersion("7.7.0"),
				podWithVersion("pod1", "7.7.0"),
				podWithVersion("pod2", "7.7.0"),
			},
			httpChecks: roundTripChecks{
				called: false,
			},
			wantUpdatedEnts: entSearchWithVersion("7.7.0", map[string]string{
				ReadOnlyModeAnnotationName: "true",
			}),
		},
		{
			name: "version upgrade over: disable read-only mode",
			ents: entSearchWithVersion("7.7.1", map[string]string{
				ReadOnlyModeAnnotationName: "true",
			}),
			runtimeObjs: []runtime.Object{
				deploymentWithVersion("7.7.1"),
				podWithVersion("pod1", "7.7.1"),
				podWithVersion("pod2", "7.7.1"),
			},
			httpChecks: roundTripChecks{
				called:           true,
				withURL:          "https://ents-ents-http.ns.svc:3002/api/ent/v1/internal/read_only_mode",
				withBody:         "{\"enabled\": false}",
				returnStatusCode: 200,
			},
			wantUpdatedEnts: entSearchWithVersion("7.7.0", map[string]string{}),
		},
		{
			name: "version upgrade requested, but no association configured : do nothing",
			ents: entsv1beta1.EnterpriseSearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ents"},
				Spec: entsv1beta1.EnterpriseSearchSpec{Version: "7.7.1"}},
			runtimeObjs: []runtime.Object{
				deploymentWithVersion("7.7.0"),
				podWithVersion("pod1", "7.7.0"),
				podWithVersion("pod2", "7.7.0"),
			},
			httpChecks: roundTripChecks{
				called: false,
			},
			wantUpdatedEnts: entsv1beta1.EnterpriseSearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ents"},
				Spec: entsv1beta1.EnterpriseSearchSpec{Version: "7.7.1"}},
		},
	}
	for _, tt := range tests {
		checks := roundTripChecks{returnStatusCode: tt.httpChecks.returnStatusCode}
		httpClient := &http.Client{Transport: fakeRoundTrip{checks: &checks}}
		k8sClient := k8s.WrappedFakeClient(append(append(tt.runtimeObjs, &esUserSecret), &tt.ents)...)
		t.Run(tt.name, func(t *testing.T) {
			r := &VersionUpgrade{
				k8sClient:  k8sClient,
				ents:       tt.ents,
				httpClient: httpClient,
				recorder:   record.NewFakeRecorder(10),
			}
			err := r.Handle(context.Background())
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err, tt.wantErr)
			}
			require.Equal(t, tt.httpChecks, checks)
		})
	}
}

func Test_hasReadOnlyAnnotationTrue(t *testing.T) {
	tests := []struct {
		name string
		ents entsv1beta1.EnterpriseSearch
		want bool
	}{
		{
			name: "annotation set to true: true",
			ents: entsv1beta1.EnterpriseSearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ents",
				Annotations: map[string]string{ReadOnlyModeAnnotationName: "true"},
			}},
			want: true,
		},
		{
			name: "no annotation set: false",
			ents: entsv1beta1.EnterpriseSearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ents",
				Annotations: nil,
			}},
			want: false,
		},
		{
			name: "annotation set to anything else: false",
			ents: entsv1beta1.EnterpriseSearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ents",
				Annotations: map[string]string{ReadOnlyModeAnnotationName: "anything-else"},
			}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasReadOnlyAnnotationTrue(tt.ents); got != tt.want {
				t.Errorf("hasReadOnlyAnnotationTrue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVersionUpgrade_isPriorVersionStillRunning(t *testing.T) {
	tests := []struct {
		name string
		ents entsv1beta1.EnterpriseSearch
		pods []runtime.Object
		want bool
	}{
		{
			name: "no Pods exist: not a version upgrade",
			ents: entSearchWithVersion("7.7.1", nil),
			pods: []runtime.Object{},
			want: false,
		},
		{
			name: "all Pods match the expected version: not a version upgrade",
			ents: entSearchWithVersion("7.7.0", nil),
			pods: []runtime.Object{
				podWithVersion("pod1", "7.7.0"),
				podWithVersion("pod2", "7.7.0"),
			},
			want: false,
		},
		{
			name: "at least one Pod has an earlier version: version upgrade",
			ents: entSearchWithVersion("7.7.1", nil),
			pods: []runtime.Object{
				podWithVersion("pod1", "7.7.1"),
				podWithVersion("pod2", "7.7.0"),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		c := k8s.WrappedFakeClient(tt.pods...)
		t.Run(tt.name, func(t *testing.T) {
			r := &VersionUpgrade{
				k8sClient: c,
				ents:      tt.ents,
			}
			got, err := r.isPriorVersionStillRunning(version.MustParse(tt.ents.Spec.Version))
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestVersionUpgrade_readOnlyModeRequest(t *testing.T) {
	ents := entSearchWithVersion("7.7.0", nil)

	tests := []struct {
		name     string
		enabled  bool
		wantURL  string
		wantBody string
	}{
		{
			name:     "read-only enabled",
			enabled:  true,
			wantURL:  "https://ents-ents-http.ns.svc:3002/api/ent/v1/internal/read_only_mode",
			wantBody: "{\"enabled\": true}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.WrappedFakeClient(&ents, &esUserSecret)
			u := &VersionUpgrade{k8sClient: c, ents: ents}
			req, err := u.readOnlyModeRequest(tt.enabled)
			require.NoError(t, err)

			// check URL
			require.Equal(t, tt.wantURL, req.URL.String())

			// check body
			body, err := ioutil.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, tt.wantBody, string(body))

			// check basic auth
			basicAuthUser, basicAuthPassword, ok := req.BasicAuth()
			require.True(t, ok)
			require.Equal(t, esUser, basicAuthUser)
			require.Equal(t, esPassword, basicAuthPassword)
		})
	}
}

func TestVersionUpgrade_isVersionUpgrade(t *testing.T) {
	entsv77 := entSearchWithVersion("7.7.0", nil)
	deploymentv77 := deploymentWithVersion("7.7.0")
	entsv78 := entSearchWithVersion("7.8.0", nil)

	tests := []struct {
		name            string
		runtimeObjs     []runtime.Object
		ents            entsv1beta1.EnterpriseSearch
		expectedVersion version.Version
		want            bool
	}{
		{
			name:            "7.7.0 to 7.7.0: not a version upgrade",
			runtimeObjs:     []runtime.Object{deploymentv77},
			ents:            entsv77,
			expectedVersion: version.MustParse(entsv77.Spec.Version),
			want:            false,
		},
		{
			name:            "7.7.0 to 7.8.0: version upgrade",
			runtimeObjs:     []runtime.Object{deploymentv77},
			ents:            entsv78,
			expectedVersion: version.MustParse(entsv78.Spec.Version),
			want:            true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &VersionUpgrade{
				k8sClient: k8s.WrappedFakeClient(tt.runtimeObjs...),
				ents:      tt.ents,
			}
			got, err := r.isVersionUpgrade(tt.expectedVersion)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
