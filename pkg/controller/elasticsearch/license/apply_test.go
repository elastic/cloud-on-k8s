// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	fixtures "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_updateLicense(t *testing.T) {
	enterpriseLicense := esclient.License{
		UID:  "enterpise-license",
		Type: string(esclient.ElasticsearchLicenseTypeEnterprise),
	}
	type args struct {
		current esclient.License
		desired esclient.License
	}
	tests := []struct {
		name    string
		args    args
		reqFn   esclient.RoundTripFunc
		wantErr bool
	}{
		{
			name:    "error: HTTP error",
			wantErr: true,
			args: args{
				desired: enterpriseLicense,
			},
			reqFn: func(req *http.Request) *http.Response {
				return esclient.NewMockResponse(400, req, "")
			},
		},
		{
			name:    "error: ES error",
			wantErr: true,
			args: args{
				desired: enterpriseLicense,
			},
			reqFn: func(req *http.Request) *http.Response {
				return esclient.NewMockResponse(
					200,
					req,
					fixtures.LicenseFailedUpdateResponseSample,
				)
			},
		},
		{
			name: "happy path",
			args: args{
				desired: enterpriseLicense,
			},
			reqFn: func(req *http.Request) *http.Response {
				return esclient.NewMockResponse(
					200,
					req,
					fixtures.LicenseUpdateResponseSample,
				)
			},
		},
		{
			name: "start a trial",
			args: args{
				desired: esclient.License{
					Type: string(esclient.ElasticsearchLicenseTypeTrial),
				},
			},
			reqFn: func(req *http.Request) *http.Response {
				if strings.Contains(req.URL.Path, "start_trial") {
					return esclient.NewMockResponse(200, req, `{"acknowledged": true, "trial_started": true}`)
				}
				panic("should only call start_trial")
			},
			wantErr: false,
		},
		{
			name: "short-circuit: already up to date",
			args: args{
				current: esclient.License{
					UID: "this-is-a-uid",
				},
				desired: esclient.License{
					UID: "this-is-a-uid",
				},
			},
			reqFn: func(req *http.Request) *http.Response {
				panic("this should never be called")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := esclient.NewMockClient(version.MustParse("6.8.0"), tt.reqFn)
			if err := updateLicense(context.Background(), types.NamespacedName{}, c, tt.args.current, tt.args.desired); (err != nil) != tt.wantErr {
				t.Errorf("updateLicense() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_applyLinkedLicense(t *testing.T) {
	clusterName := types.NamespacedName{
		Name:      "test",
		Namespace: "default",
	}
	tests := []struct {
		name             string
		initialObjs      []client.Object
		currentLicense   esclient.License
		errors           map[client.ObjectKey]error
		wantErr          bool
		clientAssertions func(updater fakeLicenseUpdater)
	}{
		{
			name:    "happy path",
			wantErr: false,
			initialObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      esv1.LicenseSecretName("test"),
						Namespace: "default",
					},
					Data: map[string][]byte{
						"anything": []byte(fixtures.LicenseSample),
					},
				},
			},
			clientAssertions: func(updater fakeLicenseUpdater) {
				require.True(t, updater.updateLicenseCalled, "should update license")
			},
		},
		{
			name:           "no error: no license found but stack has an enterprise license",
			wantErr:        false,
			currentLicense: esclient.License{Type: string(esclient.ElasticsearchLicenseTypeEnterprise)},
			clientAssertions: func(updater fakeLicenseUpdater) {
				require.True(t, updater.startBasicCalled, "should call start_basic to remove the license")
			},
		},
		{
			name:           "no error: no license found, stack already in basic license",
			wantErr:        false,
			currentLicense: esclient.License{Type: string(esclient.ElasticsearchLicenseTypeBasic)},
			clientAssertions: func(updater fakeLicenseUpdater) {
				require.False(t, updater.startBasicCalled, "should not call start_basic if already basic")
			},
		},
		{
			name:           "no error: no license found but tolerate a cluster level trial",
			wantErr:        false,
			currentLicense: esclient.License{Type: string(esclient.ElasticsearchLicenseTypeTrial)},
			clientAssertions: func(updater fakeLicenseUpdater) {
				require.False(t, updater.startBasicCalled, "should not call start_basic")
			},
		},
		{
			name:    "error: empty license",
			wantErr: true,
			initialObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      esv1.LicenseSecretName("test"),
						Namespace: "default",
					},
				},
			},
		},
		{
			name:    "error: invalid license json",
			wantErr: true,
			initialObjs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      esv1.LicenseSecretName("test"),
						Namespace: "default",
					},
					Data: map[string][]byte{
						"anything2": {},
					},
				},
			},
		},
		{
			name:    "error: request error",
			wantErr: true,
			errors: map[client.ObjectKey]error{
				types.NamespacedName{
					Namespace: clusterName.Namespace,
					Name:      esv1.LicenseSecretName("test"),
				}: errors.New("boom"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &fakeClient{
				Client: k8s.NewFakeClient(tt.initialObjs...),
				errors: tt.errors,
			}
			updater := fakeLicenseUpdater{license: tt.currentLicense}
			if err := applyLinkedLicense(
				context.Background(),
				c,
				clusterName,
				&updater,
				tt.currentLicense,
			); (err != nil) != tt.wantErr {
				t.Errorf("applyLinkedLicense() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.clientAssertions != nil {
				tt.clientAssertions(updater)
			}
		})
	}
}

func Test_checkEsLicense(t *testing.T) {
	tests := []struct {
		name        string
		wantErr     bool
		supported   bool
		esReachable bool
		updater     esclient.LicenseClient
	}{
		{
			name:        "happy path",
			wantErr:     false,
			supported:   true,
			esReachable: true,
			updater: &fakeInvalidLicenseUpdater{
				fakeLicenseUpdater:     &fakeLicenseUpdater{license: esclient.License{Type: string(esclient.ElasticsearchLicenseTypeBasic)}},
				statusCodeOnGetLicense: 200,
			},
		},
		{
			name:        "error: 400 on get license, unsupported distribution",
			wantErr:     true,
			supported:   false,
			esReachable: true,
			updater:     &fakeInvalidLicenseUpdater{statusCodeOnGetLicense: 400},
		},
		{
			name:        "error: 401 on get license",
			wantErr:     true,
			supported:   true,
			esReachable: true,
			updater:     &fakeInvalidLicenseUpdater{statusCodeOnGetLicense: 401},
		},
		{
			name:        "error: 403 on get license",
			wantErr:     true,
			supported:   true,
			esReachable: true,
			updater:     &fakeInvalidLicenseUpdater{statusCodeOnGetLicense: 403},
		},
		{
			name:        "error: 404 on get license",
			wantErr:     true,
			supported:   true,
			esReachable: true,
			updater:     &fakeInvalidLicenseUpdater{statusCodeOnGetLicense: 404},
		},
		{
			name:        "error: 500 on get license, es not reachable",
			wantErr:     true,
			supported:   true,
			esReachable: false,
			updater:     &fakeInvalidLicenseUpdater{statusCodeOnGetLicense: 500},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CheckElasticsearchLicense(context.Background(), tt.updater)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckElasticsearchLicense() error = %v, wantErr %v", err, tt.wantErr)
			}
			supported := true
			esReachable := true
			var checkErr *GetLicenseError
			if errors.As(err, &checkErr) {
				supported = checkErr.SupportedDistribution
				esReachable = checkErr.EsReachable
			}
			if supported != tt.supported {
				t.Errorf("CheckElasticsearchLicense() supported = %v, supported %v", supported, tt.supported)
			}
			if esReachable != tt.esReachable {
				t.Errorf("CheckElasticsearchLicense() esReachable = %v, esReachable %v", esReachable, tt.esReachable)
			}
		})
	}
}

type fakeInvalidLicenseUpdater struct {
	*fakeLicenseUpdater
	statusCodeOnGetLicense int
}

func (f *fakeInvalidLicenseUpdater) GetLicense(_ context.Context) (esclient.License, error) {
	if f.statusCodeOnGetLicense == 200 {
		return f.license, nil
	}
	return esclient.License{}, &esclient.APIError{StatusCode: f.statusCodeOnGetLicense}
}

type fakeLicenseUpdater struct {
	license             esclient.License
	startBasicCalled    bool
	updateLicenseCalled bool
}

func (f *fakeLicenseUpdater) StartTrial(_ context.Context) (esclient.StartTrialResponse, error) {
	return esclient.StartTrialResponse{
		Acknowledged:    true,
		TrialWasStarted: true,
	}, nil
}

func (f *fakeLicenseUpdater) GetLicense(_ context.Context) (esclient.License, error) {
	return f.license, nil
}

func (f *fakeLicenseUpdater) UpdateLicense(_ context.Context, _ esclient.LicenseUpdateRequest) (esclient.LicenseUpdateResponse, error) {
	f.updateLicenseCalled = true
	return esclient.LicenseUpdateResponse{
		Acknowledged:  true,
		LicenseStatus: "valid",
	}, nil
}

func (f *fakeLicenseUpdater) StartBasic(_ context.Context) (esclient.StartBasicResponse, error) {
	f.startBasicCalled = true
	return esclient.StartBasicResponse{}, nil
}

var _ esclient.LicenseClient = &fakeLicenseUpdater{}

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

var _ k8s.Client = &fakeClient{}
