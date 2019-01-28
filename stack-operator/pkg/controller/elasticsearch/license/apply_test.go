package license

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/pkg/errors"
	"k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
				&v1.Secret{
					ObjectMeta: v12.ObjectMeta{
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
			name:    "error: no secret found",
			wantErr: true,
		},
		{
			name:    "error: multiple keys in secret",
			wantErr: true,
			initialObjs: []runtime.Object{
				&v1.Secret{
					ObjectMeta: v12.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"1": []byte("v"),
						"2": []byte("v"),
					},
				},
			},
		},
		{
			name:    "error: empty secret",
			wantErr: true,
			initialObjs: []runtime.Object{
				&v1.Secret{
					ObjectMeta: v12.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewFakeClient(tt.initialObjs...)
			ref := v1.SecretReference{
				Name:      "test",
				Namespace: "default",
			}
			got, err := secretRefResolver(c, ref)()
			if err != nil && !tt.wantErr {
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
						UID: "this-is-a-uid",
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

			c := esclient.NewMockClient(tt.reqFn)
			if err := updateLicense(&c, tt.args.current, tt.args.desired, tt.args.sigResolver); (err != nil) != tt.wantErr {
				t.Errorf("updateLicense() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
