// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"context"
	"encoding/json"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func asJSON(obj interface{}) []byte {
	data, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return data
}

func Test_validatingWebhook_Handle(t *testing.T) {
	decoder, _ := admission.NewDecoder(k8s.Scheme())
	type fields struct {
		client               k8s.Client
		validateStorageClass bool
	}
	type args struct {
		req admission.Request
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   admission.Response
	}{
		{
			name: "accept valid creation",
			args: args{
				req: admission.Request{AdmissionRequest: v1beta1.AdmissionRequest{
					Operation: v1beta1.Create,
					Object: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
							Spec:       esv1.ElasticsearchSpec{Version: "7.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					}},
				},
			},
			want: admission.Allowed(""),
		},
		{
			name: "reject invalid creation (no version provided)",
			args: args{
				req: admission.Request{AdmissionRequest: v1beta1.AdmissionRequest{
					Operation: v1beta1.Create,
					Object: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
							Spec:       esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					}},
				},
			},
			want: admission.Denied(parseVersionErrMsg),
		},
		{
			name: "accept valid update (count++)",
			fields: fields{
				client: k8s.WrappedFakeClient(),
			},
			args: args{
				req: admission.Request{AdmissionRequest: v1beta1.AdmissionRequest{
					Operation: v1beta1.Update,
					OldObject: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
							Spec:       esv1.ElasticsearchSpec{Version: "7.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					},
					Object: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
							Spec:       esv1.ElasticsearchSpec{Version: "7.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 4}}},
						}),
					},
				}},
			},
			want: admission.Allowed(""),
		},
		{
			name: "reject invalid update (version downgrade))",
			fields: fields{
				client: k8s.WrappedFakeClient(),
			},
			args: args{
				req: admission.Request{AdmissionRequest: v1beta1.AdmissionRequest{
					Operation: v1beta1.Update,
					OldObject: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
							Spec:       esv1.ElasticsearchSpec{Version: "7.9.1", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					},
					Object: runtime.RawExtension{
						Raw: asJSON(&esv1.Elasticsearch{
							ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
							Spec:       esv1.ElasticsearchSpec{Version: "7.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
						}),
					},
				}},
			},
			want: admission.Denied(noDowngradesMsg),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wh := &validatingWebhook{
				client:               tt.fields.client,
				decoder:              decoder,
				validateStorageClass: tt.fields.validateStorageClass,
			}
			got := wh.Handle(context.Background(), tt.args.req)
			require.Equal(t, tt.want.Allowed, got.Allowed)
			if !got.Allowed {
				require.Contains(t, got.Result.Reason, tt.want.Result.Reason)
			}
		})
	}
}
