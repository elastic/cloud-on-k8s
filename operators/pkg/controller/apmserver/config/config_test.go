// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_getCredentials(t *testing.T) {
	type args struct {
		c  k8s.Client
		as v1alpha1.ApmServer
	}
	tests := []struct {
		name         string
		args         args
		wantUsername string
		wantPassword string
		wantErr      bool
	}{
		{
			name: "Test output configuration with a SecretKeyRef",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "apmelasticsearchassociation-sample-elastic-internal-apm",
						Namespace: "default",
					},
					Data: map[string][]byte{"elastic-internal-apm": []byte("a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy")},
				})),
				as: v1alpha1.ApmServer{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "apm-server-sample",
						Namespace: "default",
					},
					Spec: v1alpha1.ApmServerSpec{
						Output: v1alpha1.Output{
							Elasticsearch: v1alpha1.ElasticsearchOutput{
								Hosts: []string{"https://elasticsearch-sample-es-http.default.svc:9200"},
								Auth: v1alpha1.ElasticsearchAuth{
									SecretKeyRef: &corev1.SecretKeySelector{
										Key: "elastic-internal-apm",
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "apmelasticsearchassociation-sample-elastic-internal-apm",
										},
									},
								},
							},
						},
					},
				},
			},
			wantUsername: "elastic-internal-apm",
			wantPassword: "a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUsername, gotPassword, err := getCredentials(tt.args.c, tt.args.as)
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
