// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetCredentials(t *testing.T) {
	apmServer := &v1alpha1.ApmServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apm-server-sample",
			Namespace: "default",
		},
		Spec: v1alpha1.ApmServerSpec{
			Elasticsearch: v1alpha1.ElasticsearchOutput{
				Hosts: []string{"https://elasticsearch-sample-es-http.default.svc:9200"},
			},
		},
	}

	tests := []struct {
		name         string
		client       k8s.Client
		auth         commonv1alpha1.ElasticsearchAuth
		wantUsername string
		wantPassword string
		wantErr      bool
	}{
		{
			name: "When SecretKeyRef is defined",
			client: k8s.WrapClient(fake.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apmelasticsearchassociation-sample-elastic-internal-apm",
					Namespace: "default",
				},
				Data: map[string][]byte{"elastic-internal-apm": []byte("a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy")},
			})),
			auth: commonv1alpha1.ElasticsearchAuth{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: "elastic-internal-apm",
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "apmelasticsearchassociation-sample-elastic-internal-apm",
					},
				},
			},
			wantUsername: "elastic-internal-apm",
			wantPassword: "a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy",
		},
		{
			name: "When SecretKeyRef is undefined",
			client: k8s.WrapClient(fake.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apmelasticsearchassociation-sample-elastic-internal-apm",
					Namespace: "default",
				},
				Data: map[string][]byte{"elastic-internal-apm": []byte("a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy")},
			})),
			auth:         commonv1alpha1.ElasticsearchAuth{},
			wantUsername: "",
			wantPassword: "",
		},
		{
			name: "When the secret does not exist",
			client: k8s.WrapClient(fake.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{"elastic-internal-apm": []byte("a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy")},
			})),
			auth:         commonv1alpha1.ElasticsearchAuth{},
			wantUsername: "",
			wantPassword: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apmServer.Spec.Elasticsearch.Auth = tt.auth
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
