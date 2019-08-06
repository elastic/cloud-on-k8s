// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"reflect"
	"testing"

	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/admission/v1beta1"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

func setupScheme(t *testing.T) *runtime.Scheme {
	sc := scheme.Scheme
	if err := estype.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to add Es types")
	}
	return sc
}

type mockDecoder struct {
	err error
	obj runtime.Object
}

func (m mockDecoder) Decode(_ types.Request, o runtime.Object) error {
	if m.obj != nil {
		reflect.ValueOf(o).Elem().Set(reflect.ValueOf(m.obj).Elem())
	}
	return m.err
}

func TestValidationHandler_Handle(t *testing.T) {
	sc := setupScheme(t)
	type fields struct {
		initialObjects []runtime.Object
		decoder        types.Decoder
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
			name: "6.0.0 is not allowed",
			args: args{
				ctx: nil,
				r: types.Request{
					AdmissionRequest: &v1beta1.AdmissionRequest{
						Name:      "foo",
						Namespace: "default",
					},
				},
			},
			fields: fields{
				decoder: mockDecoder{
					obj: &estype.Elasticsearch{
						TypeMeta: v1.TypeMeta{},
						ObjectMeta: v1.ObjectMeta{
							Name:      "foo",
							Namespace: "default",
						},
						Spec: estype.ElasticsearchSpec{
							Version: "6.0.0",
							Nodes: []estype.NodeSpec{
								{
									Name:      "foo",
									NodeCount: 3,
								},
							},
						},
						Status: estype.ElasticsearchStatus{},
					},
				},
				initialObjects: []runtime.Object{},
			},
			want: types.Response{
				Response: &admissionv1beta1.AdmissionResponse{
					Allowed: false,
					Result: &v1.Status{
						Reason: "unsupported version: 6.0.0",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewFakeClientWithScheme(sc, tt.fields.initialObjects...)
			v := &ValidationHandler{
				client:  client,
				decoder: tt.fields.decoder,
			}
			if got := v.Handle(tt.args.ctx, tt.args.r); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ValidationHandler.Handle() = %v, want %v", got, tt.want)
			}
		})
	}
}
