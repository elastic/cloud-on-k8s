// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"context"
	"net/http"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	fixtures "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_updateLicense(t *testing.T) {
	type args struct {
		current *esclient.License
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
				current: nil,
				desired: esclient.License{},
			},
			reqFn: func(req *http.Request) *http.Response {
				return esclient.NewMockResponse(400, req, "")
			},
		},
		{
			name:    "error: ES error",
			wantErr: true,
			args: args{
				current: nil,
				desired: esclient.License{},
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
				current: nil,
				desired: esclient.License{},
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
			name: "short-circuit: already up to date",
			args: args{
				current: &esclient.License{
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
			if err := updateLicense(c, tt.args.current, tt.args.desired); (err != nil) != tt.wantErr {
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
		initialObjs      []runtime.Object
		errors           map[client.ObjectKey]error
		wantErr          bool
		clientAssertions func(updater fakeLicenseUpdater)
	}{
		{
			name:    "happy path",
			wantErr: false,
			initialObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      v1beta1.LicenseSecretName("test"),
						Namespace: "default",
					},
					Data: map[string][]byte{
						"anything": []byte(fixtures.LicenseSample),
					},
				},
			},
		},
		{
			name:    "no error: no license found",
			wantErr: false,
			clientAssertions: func(updater fakeLicenseUpdater) {
				require.True(t, updater.startBasicCalled, "should call start_basic")
			},
		},
		{
			name:    "error: empty license",
			wantErr: true,
			initialObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      v1beta1.LicenseSecretName("test"),
						Namespace: "default",
					},
				},
			},
		},
		{
			name:    "error: invalid license json",
			wantErr: true,
			initialObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      v1beta1.LicenseSecretName("test"),
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
					Name:      v1beta1.LicenseSecretName("test"),
				}: errors.New("boom"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &fakeClient{
				Client: k8s.WrappedFakeClient(tt.initialObjs...),
				errors: tt.errors,
			}
			updater := fakeLicenseUpdater{}
			if err := applyLinkedLicense(
				c,
				clusterName,
				nil,
				&updater,
			); (err != nil) != tt.wantErr {
				t.Errorf("applyLinkedLicense() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.clientAssertions != nil {
				tt.clientAssertions(updater)
			}
		})
	}
}

type fakeLicenseUpdater struct {
	license          esclient.License
	startBasicCalled bool
}

func (f *fakeLicenseUpdater) StartTrial(ctx context.Context) (esclient.StartTrialResponse, error) {
	return esclient.StartTrialResponse{
		Acknowledged:    true,
		TrialWasStarted: true,
	}, nil
}

func (f *fakeLicenseUpdater) GetLicense(ctx context.Context) (esclient.License, error) {
	return f.license, nil
}

func (f *fakeLicenseUpdater) UpdateLicense(ctx context.Context, licenses esclient.LicenseUpdateRequest) (esclient.LicenseUpdateResponse, error) {
	return esclient.LicenseUpdateResponse{
		Acknowledged:  true,
		LicenseStatus: "valid",
	}, nil
}

func (f *fakeLicenseUpdater) StartBasic(ctx context.Context) (esclient.StartBasicResponse, error) {
	f.startBasicCalled = true
	return esclient.StartBasicResponse{}, nil
}

var _ esclient.LicenseUpdater = &fakeLicenseUpdater{}

type fakeClient struct {
	k8s.Client
	errors map[client.ObjectKey]error
}

func (f *fakeClient) Get(key client.ObjectKey, obj runtime.Object) error {
	err := f.errors[key]
	if err != nil {
		return err
	}
	return f.Client.Get(key, obj)
}

var _ k8s.Client = &fakeClient{}
