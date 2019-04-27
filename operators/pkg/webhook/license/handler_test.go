// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis"
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

type mockDecoder struct {
	err error
	obj *v1alpha1.EnterpriseLicense
}

func (m mockDecoder) Decode(_ types.Request, o runtime.Object) error {
	if m.obj != nil {
		reflect.ValueOf(o).Elem().Set(reflect.ValueOf(m.obj).Elem())
	}
	return m.err
}

func TestValidationHandler_Handle(t *testing.T) {
	type fields struct {
		client  client.Client
		decoder types.Decoder
	}
	type args struct {
		ctx context.Context
		r   types.Request
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   types.Response
	}{
		{
			name: "not-found: OK",
			fields: fields{
				client: fake.NewFakeClient(),
				decoder: mockDecoder{
					obj: &v1alpha1.EnterpriseLicense{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							Type: v1alpha1.LicenseTypeEnterpriseTrial,
							Eula: v1alpha1.EulaState{
								Accepted: true,
							},
						},
					},
				},
			},
			args: args{
				ctx: context.TODO(),
				r: types.Request{
					AdmissionRequest: &v1beta1.AdmissionRequest{
						Operation: v1beta1.Create,
					},
				},
			},
			want: admission.ValidationResponse(true, ""),
		},
		{
			name: "fail-on-decode: FAIL",
			fields: fields{
				client: fake.NewFakeClient(),
				decoder: mockDecoder{
					err: errors.New("failed to decode"),
				},
			},
			args: args{
				ctx: nil,
				r: types.Request{
					AdmissionRequest: &v1beta1.AdmissionRequest{
						Operation: v1beta1.Create,
					},
				},
			},
			want: admission.ErrorResponse(http.StatusBadRequest, errors.New("failed to decode")),
		},
		{
			name: "invalid request: REJECT",
			fields: fields{
				client: fake.NewFakeClient(),
				decoder: mockDecoder{
					obj: &v1alpha1.EnterpriseLicense{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							Type: v1alpha1.LicenseTypeEnterpriseTrial,
						},
					},
				},
			},
			args: args{
				ctx: context.TODO(),
				r: types.Request{
					AdmissionRequest: &v1beta1.AdmissionRequest{
						Operation: v1beta1.Update,
					},
				},
			},
			want: admission.ValidationResponse(false, "Please set the field eula.accepted to true to accept the EULA"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, apis.AddToScheme(scheme.Scheme))
			v := &ValidationHandler{
				client:  tt.fields.client,
				decoder: tt.fields.decoder,
			}
			if got := v.Handle(tt.args.ctx, tt.args.r); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ValidationHandler.Handle() = %v, want %v", got, tt.want)
			}
		})
	}
}
