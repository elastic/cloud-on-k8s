// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	esclient "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	fixtures "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_secretRefResolver(t *testing.T) {
	tests := []struct {
		name        string
		initialObjs []runtime.Object
		want        string
		wantErr     bool
	}{
		{
			name: "happy-path: exactly one sig",
			initialObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"k": []byte("v"),
					},
				},
			},
			want:    "v",
			wantErr: false,
		},
		{
			name:    "happy-path: multiple keys in secret",
			wantErr: false,
			want:    "v",
			initialObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"k":     []byte("v"),
						"other": []byte("other"),
					},
				},
			},
		},
		{
			name:    "error: no secret found",
			wantErr: true,
		},

		{
			name:    "error: empty secret",
			wantErr: true,
			initialObjs: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.WrapClient(fake.NewFakeClient(tt.initialObjs...))
			ref := corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "test",
				},
				Key: "k",
			}
			got, err := secretRefResolver(c, "default", ref)()
			if (err != nil && !tt.wantErr) || err == nil && tt.wantErr {
				t.Errorf("secretRefResolver() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("secretRefResolver() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_updateLicense(t *testing.T) {
	defaultSigResolver := func() (string, error) {
		return "signature", nil
	}
	type args struct {
		current     *esclient.License
		desired     v1alpha1.ClusterLicense
		sigResolver func() (string, error)
	}
	tests := []struct {
		name    string
		args    args
		reqFn   esclient.RoundTripFunc
		wantErr bool
	}{
		{
			name:    "error: no signature",
			wantErr: true,
			args: args{
				current: nil,
				desired: v1alpha1.ClusterLicense{},
				sigResolver: func() (s string, e error) {
					return "", errors.New("boom")
				},
			},
		},
		{
			name:    "error: HTTP error",
			wantErr: true,
			args: args{
				current:     nil,
				desired:     v1alpha1.ClusterLicense{},
				sigResolver: defaultSigResolver,
			},
			reqFn: func(req *http.Request) *http.Response {
				return esclient.NewMockResponse(400, req, "")
			},
		},
		{
			name:    "error: ES error",
			wantErr: true,
			args: args{
				current:     nil,
				desired:     v1alpha1.ClusterLicense{},
				sigResolver: defaultSigResolver,
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
				current:     nil,
				desired:     v1alpha1.ClusterLicense{},
				sigResolver: defaultSigResolver,
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
				desired: v1alpha1.ClusterLicense{
					Spec: v1alpha1.ClusterLicenseSpec{
						LicenseMeta: v1alpha1.LicenseMeta{
							UID: "this-is-a-uid",
						},
					},
				},
				sigResolver: defaultSigResolver,
			},
			reqFn: func(req *http.Request) *http.Response {
				panic("this should never be called")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			c := esclient.NewMockClient(version.MustParse("6.7.0"), tt.reqFn)
			if err := updateLicense(&c, tt.args.current, tt.args.desired, tt.args.sigResolver); (err != nil) != tt.wantErr {
				t.Errorf("updateLicense() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

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

func registerScheme(t *testing.T) *runtime.Scheme {
	sc := scheme.Scheme
	if err := v1alpha1.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to build custom scheme")
	}
	return sc
}

func Test_applyLinkedLicense(t *testing.T) {
	clusterName := types.NamespacedName{
		Name:      "test-license",
		Namespace: "default",
	}
	tests := []struct {
		name        string
		initialObjs []runtime.Object
		errors      map[client.ObjectKey]error
		wantErr     bool
	}{
		{
			name:    "happy path",
			wantErr: false,
			initialObjs: []runtime.Object{
				&v1alpha1.ClusterLicense{
					ObjectMeta: k8s.ToObjectMeta(clusterName),
					Spec: v1alpha1.ClusterLicenseSpec{
						LicenseMeta: v1alpha1.LicenseMeta{
							UID: "some-uid",
						},
						Type: v1alpha1.LicenseTypePlatinum,
					},
				},
			},
		},
		{
			name:    "no error: no license found",
			wantErr: false,
		},
		{
			name:    "error: empty license",
			wantErr: true,
			initialObjs: []runtime.Object{
				&v1alpha1.ClusterLicense{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-license",
						Namespace: "default",
					},
				},
			},
		},
		{
			name:    "error: request error",
			wantErr: true,
			errors: map[client.ObjectKey]error{
				clusterName: errors.New("boom"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &fakeClient{
				Client: k8s.WrapClient(fake.NewFakeClientWithScheme(registerScheme(t), tt.initialObjs...)),
				errors: tt.errors,
			}
			if err := applyLinkedLicense(
				c,
				clusterName,
				func(license v1alpha1.ClusterLicense) error {
					return nil
				},
			); (err != nil) != tt.wantErr {
				t.Errorf("applyLinkedLicense() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
