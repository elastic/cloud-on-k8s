// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1alpha1"
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
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
		Spec: v1alpha1.ApmServerSpec{},
	}

	apmServer.SetAssociationConf(&commonv1alpha1.AssociationConf{
		URL: "https://elasticsearch-sample-es-http.default.svc:9200",
	})

	tests := []struct {
		name         string
		client       k8s.Client
		assocConf    commonv1alpha1.AssociationConf
		wantUsername string
		wantPassword string
		wantErr      bool
	}{
		{
			name: "When auth details are defined",
			client: k8s.WrapClient(fake.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apmelasticsearchassociation-sample-elastic-internal-apm",
					Namespace: "default",
				},
				Data: map[string][]byte{"elastic-internal-apm": []byte("a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy")},
			})),
			assocConf: commonv1alpha1.AssociationConf{
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
			client: k8s.WrapClient(fake.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apmelasticsearchassociation-sample-elastic-internal-apm",
					Namespace: "default",
				},
				Data: map[string][]byte{"elastic-internal-apm": []byte("a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy")},
			})),
			assocConf: commonv1alpha1.AssociationConf{
				CASecretName: "ca-secret",
				URL:          "https://elasticsearch-sample-es-http.default.svc:9200",
			},
		},
		{
			name: "When the auth secret does not exist",
			client: k8s.WrapClient(fake.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{"elastic-internal-apm": []byte("a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy")},
			})),
			assocConf: commonv1alpha1.AssociationConf{
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
