// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
)

func TestBackendElasticsearch_IsConfigured(t *testing.T) {
	caSecretName := "ca-dummy"
	type fields struct {
		URL                    string
		Auth                   ElasticsearchAuth
		CertificateAuthorities v1alpha1.SecretRef
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "empty backend is not configured",
			fields: fields{
				Auth: ElasticsearchAuth{},
			},
			want: false,
		},
		{
			name: "some fields missing is not configured",
			fields: fields{
				URL: "i am an url",
				Auth: ElasticsearchAuth{
					Inline: &ElasticsearchInlineAuth{
						Username: "foo",
						Password: "bar",
					},
				},
			},
			want: false,
		},
		{
			name: "all fields configured",
			fields: fields{
				URL: "i am an url",
				Auth: ElasticsearchAuth{
					Inline: &ElasticsearchInlineAuth{
						Username: "foo",
						Password: "bar",
					},
				},
				CertificateAuthorities: v1alpha1.SecretRef{SecretName: caSecretName},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := BackendElasticsearch{
				URL:                    tt.fields.URL,
				Auth:                   tt.fields.Auth,
				CertificateAuthorities: tt.fields.CertificateAuthorities,
			}
			if got := b.IsConfigured(); got != tt.want {
				t.Errorf("BackendElasticsearch.IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}
