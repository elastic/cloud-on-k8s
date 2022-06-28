// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package rbac

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	authorizationapi "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
)

type fakeClientProvider func() kubernetes.Interface

func Test_subjectAccessReviewer_AccessAllowed(t *testing.T) {
	es := &esv1.Elasticsearch{
		TypeMeta: metav1.TypeMeta{
			Kind: esv1.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "es",
			Namespace: "elasticsearch-ns",
		},
	}

	type fields struct {
		clientProvider fakeClientProvider
	}
	type args struct {
		serviceAccount  string
		sourceNamespace string
		object          runtime.Object
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "allowed",
			args: args{
				sourceNamespace: "kibana-ns",
				object:          es,
			},
			fields: fields{
				clientProvider: func() kubernetes.Interface {
					fakeClient := fake.NewSimpleClientset()
					fakeClient.PrependReactor(
						"create",
						"subjectaccessreviews",
						func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
							object := action.(k8stesting.CreateAction).GetObject().DeepCopyObject() //nolint:forcetypeassert
							if t, ok := object.(*authorizationapi.SubjectAccessReview); ok {
								t.Status.Allowed = true
								t.Status.Denied = false
							}
							return true, object, nil
						},
					)
					return fakeClient
				},
			},
			want: true,
		},
		{
			name: "allowed if in the same namespace",
			args: args{
				sourceNamespace: "kibana-ns",
				object: &esv1.Elasticsearch{
					TypeMeta: metav1.TypeMeta{
						Kind: esv1.Kind,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es",
						Namespace: "kibana-ns",
					},
				},
			},
			fields: fields{
				clientProvider: func() kubernetes.Interface {
					fakeClient := fake.NewSimpleClientset()
					fakeClient.PrependReactor(
						"create",
						"subjectaccessreviews",
						func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
							object := action.(k8stesting.CreateAction).GetObject().DeepCopyObject() //nolint:forcetypeassert
							if t, ok := object.(*authorizationapi.SubjectAccessReview); ok {
								t.Status.Denied = true
								t.Status.Allowed = true
							}
							return true, object, nil
						},
					)
					return fakeClient
				},
			},
			want: true,
		},
		{
			name: "not allowed",
			args: args{
				sourceNamespace: "kibana-ns",
				object:          es,
			},
			fields: fields{
				clientProvider: func() kubernetes.Interface {
					fakeClient := fake.NewSimpleClientset()
					fakeClient.PrependReactor(
						"create",
						"subjectaccessreviews",
						func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
							object := action.(k8stesting.CreateAction).GetObject().DeepCopyObject() //nolint:forcetypeassert
							if t, ok := object.(*authorizationapi.SubjectAccessReview); ok {
								t.Status.Allowed = false
								t.Status.Denied = false
							}
							return true, object, nil
						},
					)
					return fakeClient
				},
			},
			want: false,
		},
		{
			name: "not allowed in case of an error",
			args: args{
				sourceNamespace: "kibana-ns",
				object:          es,
			},
			fields: fields{
				clientProvider: func() kubernetes.Interface {
					fakeClient := fake.NewSimpleClientset()
					fakeClient.PrependReactor(
						"create",
						"subjectaccessreviews",
						func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
							object := action.(k8stesting.CreateAction).GetObject().DeepCopyObject() //nolint:forcetypeassert
							if t, ok := object.(*authorizationapi.SubjectAccessReview); ok {
								t.Status.Allowed = false
								t.Status.Denied = false
							}
							return true, object, fmt.Errorf("not allowed")
						},
					)
					return fakeClient
				},
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "explicitly denied",
			args: args{
				sourceNamespace: "kibana-ns",
				object:          es,
			},
			fields: fields{
				clientProvider: func() kubernetes.Interface {
					fakeClient := fake.NewSimpleClientset()
					fakeClient.PrependReactor(
						"create",
						"subjectaccessreviews",
						func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
							object := action.(k8stesting.CreateAction).GetObject().DeepCopyObject() //nolint:forcetypeassert
							if t, ok := object.(*authorizationapi.SubjectAccessReview); ok {
								t.Status.Denied = true
								t.Status.Allowed = true
							}
							return true, object, nil
						},
					)
					return fakeClient
				},
			},
			want: false,
		},
		{
			name: "badly formatted service account",
			args: args{
				sourceNamespace: "kibana-ns",
				object:          es,
				serviceAccount:  "system:serviceaccount:foo:bar",
			},
			fields: fields{
				clientProvider: func() kubernetes.Interface {
					return fake.NewSimpleClientset()
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SubjectAccessReviewer{
				client: tt.fields.clientProvider(),
			}
			got, err := s.AccessAllowed(context.Background(), tt.args.serviceAccount, tt.args.sourceNamespace, tt.args.object)
			if (err != nil) != tt.wantErr {
				t.Errorf("SubjectAccessReviewer.AccessAllowed() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("SubjectAccessReviewer.AccessAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_newSubjectAccessReview(t *testing.T) {
	es := &esv1.Elasticsearch{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Elasticsearch",
			APIVersion: "elasticsearch.k8s.elastic.co/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "es-sample",
			Namespace: "es-ns",
		},
	}
	type args struct {
		metaObject      metav1.Object
		object          runtime.Object
		serviceAccount  string
		sourceNamespace string
	}
	tests := []struct {
		name string
		args args
		want *authorizationapi.SubjectAccessReview
	}{
		{
			name: "Simple SubjectAccessReview generation",
			args: args{
				object:          es,
				metaObject:      es,
				serviceAccount:  "foo",
				sourceNamespace: "apmserver-ns",
			},
			want: &authorizationapi.SubjectAccessReview{
				Spec: authorizationapi.SubjectAccessReviewSpec{
					ResourceAttributes: &authorizationapi.ResourceAttributes{
						Namespace: "es-ns",
						Verb:      "get",
						Resource:  "elasticsearches",
						Group:     "elasticsearch.k8s.elastic.co",
						Version:   "v1",
						Name:      "es-sample",
					},
					User: ServiceAccountUsernamePrefix + "apmserver-ns" + ":" + "foo",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newSubjectAccessReview(tt.args.metaObject, tt.args.object, tt.args.serviceAccount, tt.args.sourceNamespace); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newSubjectAccessReview() = %v, want %v", got, tt.want)
			}
		})
	}
}
