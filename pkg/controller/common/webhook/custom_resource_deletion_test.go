// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package webhook

import (
	"context"
	"reflect"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_crdDeletionWebhook_Handle(t *testing.T) {
	tests := []struct {
		name   string
		client k8s.Client
		req    admission.Request
		want   admission.Response
	}{
		{
			name: "elasticsearch crd deletion is prevented when es instance exists",
			client: fake.NewClientBuilder().
				WithObjects(&esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Name: "es", Namespace: "test"},
				}).
				WithScheme(k8s.Scheme()).
				Build(),
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					// 'OldObject' is populated with a Delete operation, not 'Object'.
					OldObject: runtime.RawExtension{
						Raw: asJSON(&extensionsv1.CustomResourceDefinition{
							ObjectMeta: metav1.ObjectMeta{
								Name: "elasticsearches.elasticsearch.k8s.elastic.co",
							},
							Spec: extensionsv1.CustomResourceDefinitionSpec{
								Group: esv1.GroupVersion.Group,
								Names: extensionsv1.CustomResourceDefinitionNames{
									Kind: esv1.Kind,
								},
								Versions: []extensionsv1.CustomResourceDefinitionVersion{
									{Name: "v1"},
								},
							},
						}),
					},
				},
			},
			want: admission.Denied("deletion of Elastic CRDs is not allowed while in use"),
		},
		{
			name:   "elasticsearch crd deletion is allowed when no es instance exists",
			client: fake.NewClientBuilder().WithObjects().WithScheme(k8s.Scheme()).Build(),
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					// 'OldObject' is populated with a Delete operation, not 'Object'.
					OldObject: runtime.RawExtension{
						Raw: asJSON(&extensionsv1.CustomResourceDefinition{
							ObjectMeta: metav1.ObjectMeta{
								Name: "elasticsearches.elasticsearch.k8s.elastic.co",
							},
							Spec: extensionsv1.CustomResourceDefinitionSpec{
								Group: "elasticsearch.k8s.elastic.co",
								Names: extensionsv1.CustomResourceDefinitionNames{
									Kind: "Elasticsearch",
								},
								Versions: []extensionsv1.CustomResourceDefinitionVersion{
									{Name: "v1"},
								},
							},
						}),
					},
				},
			},
			want: admission.Allowed(""),
		},
		{
			name:   "Non-Elastic CRD deletion is allowed",
			client: fake.NewClientBuilder().WithObjects().WithScheme(k8s.Scheme()).Build(),
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					// 'OldObject' is populated with a Delete operation, not 'Object'.
					OldObject: runtime.RawExtension{
						Raw: asJSON(&extensionsv1.CustomResourceDefinition{
							ObjectMeta: metav1.ObjectMeta{
								Name: "clusterpolicies.kyverno.io",
							},
							Spec: extensionsv1.CustomResourceDefinitionSpec{
								Group: "kyverno.io",
								Names: extensionsv1.CustomResourceDefinitionNames{
									Kind: "ClusterPolicy",
								},
								Versions: []extensionsv1.CustomResourceDefinitionVersion{
									{Name: "v1"},
								},
							},
						}),
					},
				},
			},
			want: admission.Allowed(""),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			decoder := admission.NewDecoder(k8s.Scheme())
			wh := &crdDeletionWebhook{
				client:  tt.client,
				decoder: decoder,
			}
			if got := wh.Handle(ctx, tt.req); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("crdDeletionWebhook.Handle() = %v, want %v", got, tt.want)
			}
		})
	}
}
