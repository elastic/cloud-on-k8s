// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package rbac

import (
	"fmt"
	"reflect"
	"testing"

	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	authorizationapi "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

type fakeClientProvider func() kubernetes.Interface

func Test_subjectAccessReviewer_AccessAllowed(t *testing.T) {

	es := &v1.Elasticsearch{
		TypeMeta: metav1.TypeMeta{
			Kind: "Elasticsearch",
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
							object := action.(k8stesting.CreateAction).GetObject().DeepCopyObject()
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
				object: &v1.Elasticsearch{
					TypeMeta: metav1.TypeMeta{
						Kind: "Elasticsearch",
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
							object := action.(k8stesting.CreateAction).GetObject().DeepCopyObject()
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
							object := action.(k8stesting.CreateAction).GetObject().DeepCopyObject()
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
							object := action.(k8stesting.CreateAction).GetObject().DeepCopyObject()
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
							object := action.(k8stesting.CreateAction).GetObject().DeepCopyObject()
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
			s := &subjectAccessReviewer{
				client: tt.fields.clientProvider(),
			}
			got, err := s.AccessAllowed(tt.args.serviceAccount, tt.args.sourceNamespace, tt.args.object)
			if (err != nil) != tt.wantErr {
				t.Errorf("subjectAccessReviewer.AccessAllowed() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("subjectAccessReviewer.AccessAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNextReconciliation(t *testing.T) {
	type args struct {
		accessReviewer AccessReviewer
	}
	tests := []struct {
		name                string
		args                args
		wantNonZeroDuration bool
	}{
		{
			name:                "Schedule a requeue if there's some access control",
			args:                args{accessReviewer: NewSubjectAccessReviewer(fake.NewSimpleClientset())},
			wantNonZeroDuration: true,
		},
		{
			name:                "No requeue if there is no access control",
			args:                args{accessReviewer: NewPermissiveAccessReviewer()},
			wantNonZeroDuration: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NextReconciliation(tt.args.accessReviewer); !reflect.DeepEqual(got.RequeueAfter > 0, tt.wantNonZeroDuration) {
				t.Errorf("NextReconciliation() = %v, wantNonZeroDuration: %v", got, tt.wantNonZeroDuration)
			}
		})
	}
}
