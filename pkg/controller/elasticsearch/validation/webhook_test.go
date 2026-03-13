// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonwebhook "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func asJSON(obj any) []byte {
	data, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return data
}

func Test_validator_Handle(t *testing.T) {
	type fields struct {
		client               k8s.Client
		validateStorageClass bool
	}
	tests := []struct {
		name        string
		fields      fields
		req         admission.Request
		wantAllowed bool
		wantMessage string
	}{
		{
			name: "accept valid creation",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				}},
			},
			wantAllowed: true,
		},
		{
			name: "request from un-managed namespace is ignored, and just accepted",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "unmanaged", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				}},
			},
			wantAllowed: true,
		},
		{
			name: "reject invalid creation (no version provided)",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
						Spec:       esv1.ElasticsearchSpec{NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				}},
			},
			wantAllowed: false,
			wantMessage: parseVersionErrMsg,
		},
		{
			name: "accept valid update (count++)",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Update,
				OldObject: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				},
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 4}}},
					}),
				},
			}},
			wantAllowed: true,
		},
		{
			name: "reject invalid update (version downgrade))",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Update,
				OldObject: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "8.9.1", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				},
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "8.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				},
			}},
			wantAllowed: false,
			wantMessage: noDowngradesMsg,
		},
		{
			name: "reject invalid update (from 8.9.0 to 9.0.0))",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Update,
				OldObject: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "8.9.1", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				},
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "9.0.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				},
			}},
			wantAllowed: false,
			wantMessage: unsupportedUpgradeMsg,
		},
		{
			name: "accept valid update (from 8.18.0 to 9.0.0))",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Update,
				OldObject: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "8.18.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				},
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "9.0.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				},
			}},
			wantAllowed: true,
		},
		{
			name: "accept valid creation with warnings due to deprecated version",
			fields: fields{
				client: k8s.NewFakeClient(),
			},
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: asJSON(&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"},
						Spec:       esv1.ElasticsearchSpec{Version: "7.9.0", NodeSets: []esv1.NodeSet{{Name: "set1", Count: 3}}},
					}),
				}},
			},
			wantAllowed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := &validator{
				client:               tt.fields.client,
				validateStorageClass: tt.fields.validateStorageClass,
			}
			v := commonwebhook.NewResourceValidator[*esv1.Elasticsearch](nil, []string{"ns"}, inner)
			wh := admission.WithValidator[*esv1.Elasticsearch](k8s.Scheme(), v)
			got := wh.Handle(context.Background(), tt.req)
			require.Equal(t, tt.wantAllowed, got.Allowed)
			if !got.Allowed {
				require.Contains(t, got.Result.Message, tt.wantMessage)
			}
		})
	}
}
