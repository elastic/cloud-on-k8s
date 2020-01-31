// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"testing"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestGetCredentials(t *testing.T) {
	apmServer := &apmv1.ApmServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apm-server-sample",
			Namespace: "default",
		},
		Spec: apmv1.ApmServerSpec{},
	}

	apmServer.SetAssociationConf(&commonv1.AssociationConf{
		URL: "https://elasticsearch-sample-es-http.default.svc:9200",
	})

	tests := []struct {
		name         string
		client       k8s.Client
		assocConf    commonv1.AssociationConf
		wantUsername string
		wantPassword string
		wantErr      bool
	}{
		{
			name: "When auth details are defined",
			client: k8s.WrappedFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apmelasticsearchassociation-sample-elastic-internal-apm",
					Namespace: "default",
				},
				Data: map[string][]byte{"elastic-internal-apm": []byte("a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy")},
			}),
			assocConf: commonv1.AssociationConf{
				AuthSecretName: "apmelasticsearchassociation-sample-elastic-internal-apm",
				AuthSecretKey:  "elastic-internal-apm",
				CASecretName:   "ca-secret",
				URL:            "https://elasticsearch-sample-es-http.default.svc:9200",
			},
			wantUsername: "elastic-internal-apm",
			wantPassword: "a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy",
		},
		{
			name: "When auth details are undefined",
			client: k8s.WrappedFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apmelasticsearchassociation-sample-elastic-internal-apm",
					Namespace: "default",
				},
				Data: map[string][]byte{"elastic-internal-apm": []byte("a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy")},
			}),
			assocConf: commonv1.AssociationConf{
				CASecretName: "ca-secret",
				URL:          "https://elasticsearch-sample-es-http.default.svc:9200",
			},
		},
		{
			name: "When the auth secret does not exist",
			client: k8s.WrappedFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{"elastic-internal-apm": []byte("a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy")},
			}),
			assocConf: commonv1.AssociationConf{
				AuthSecretName: "apmelasticsearchassociation-sample-elastic-internal-apm",
				AuthSecretKey:  "elastic-internal-apm",
				CASecretName:   "ca-secret",
				URL:            "https://elasticsearch-sample-es-http.default.svc:9200",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apmServer.SetAssociationConf(&tt.assocConf)
			gotUsername, gotPassword, err := ElasticsearchAuthSettings(tt.client, apmServer)
			if (err != nil) != tt.wantErr {
				t.Errorf("getCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotUsername != tt.wantUsername {
				t.Errorf("getCredentials() gotUsername = %v, want %v", gotUsername, tt.wantUsername)
			}
			if gotPassword != tt.wantPassword {
				t.Errorf("getCredentials() gotPassword = %v, want %v", gotPassword, tt.wantPassword)
			}
		})
	}
}

type fakeAccessReviewer struct {
	allowed bool
	err     error
}

func (f *fakeAccessReviewer) AccessAllowed(_ string, _ string, _ runtime.Object) (bool, error) {
	return f.allowed, f.err
}

type fakeUnbinder struct {
	called bool
}

func (f *fakeUnbinder) Unbind(associated commonv1.Associated) error {
	f.called = true
	return nil
}

func TestIsAllowedReference(t *testing.T) {
	apmServer := &apmv1.ApmServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apm-server-sample",
			Namespace: "apmserver-ns",
		},
		Spec: apmv1.ApmServerSpec{},
	}
	es := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "es-sample",
			Namespace: "es-ns",
		},
	}

	type args struct {
		accessReviewer rbac.AccessReviewer
		associated     commonv1.Associated
		object         runtime.Object
		unbinder       fakeUnbinder
	}
	tests := []struct {
		name                                  string
		args                                  args
		want, wantErr, wantFakeUnbinderCalled bool
	}{
		{
			name: "Association not allowed, ensure unbinder is called",
			args: args{
				associated: apmServer,
				object:     es,
				accessReviewer: &fakeAccessReviewer{
					allowed: false,
				},
				unbinder: fakeUnbinder{},
			},
			wantFakeUnbinderCalled: true,
			want:                   false,
		},
		{
			name: "Association allowed, ensure unbinder is not called",
			args: args{
				associated: apmServer,
				object:     es,
				accessReviewer: &fakeAccessReviewer{
					allowed: true,
				},
				unbinder: fakeUnbinder{},
			},
			wantFakeUnbinderCalled: false,
			want:                   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsAllowedReference(tt.args.accessReviewer, tt.args.associated, tt.args.object, &tt.args.unbinder)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsAllowedReference() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IsAllowedReference() = %v, want %v", got, tt.want)
			}
			if tt.args.unbinder.called != tt.wantFakeUnbinderCalled {
				t.Errorf("fakeUnbinder.called = %v, want %v", tt.args.unbinder.called, tt.wantFakeUnbinderCalled)
			}
		})
	}
}
