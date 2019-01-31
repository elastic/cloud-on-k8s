package license

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

type fakeReader struct {
	fakeClient client.Client
	errors     map[client.ObjectKey]error
}

func (f *fakeReader) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	err := f.errors[key]
	if err != nil {
		return err
	}
	return f.fakeClient.Get(ctx, key, obj)
}

func (f *fakeReader) List(ctx context.Context, opts *client.ListOptions, list runtime.Object) error {
	return f.fakeClient.List(ctx, opts, list)
}

var _ client.Reader = &fakeReader{}

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
						UID:  "some-uid",
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
					ObjectMeta: v12.ObjectMeta{
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
			c := &fakeReader{
				fakeClient: fake.NewFakeClientWithScheme(registerScheme(t), tt.initialObjs...),
				errors:     tt.errors,
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
