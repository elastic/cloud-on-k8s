// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package services

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExternalServiceURL(t *testing.T) {
	type args struct {
		es v1alpha1.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "A service URL (basic license)",
			args: args{es: v1alpha1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "an-es-name",
					Namespace: "default",
				},
			}},
			want: "http://an-es-name-es.default.svc.cluster.local:9200",
		},
		{
			name: "Another Service URL (basic license)",
			args: args{es: v1alpha1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "another-es-name",
					Namespace: "default",
				},
			}},
			want: "http://another-es-name-es.default.svc.cluster.local:9200",
		},
		{
			name: "A service URL with trial license",
			args: args{es: v1alpha1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "an-es-name",
					Namespace: "default",
				},
				Spec: v1alpha1.ElasticsearchSpec{
					LicenseType: "trial",
				},
			}},
			want: "https://an-es-name-es.default.svc.cluster.local:9200",
		},
		{
			name: "A service URL with gold license",
			args: args{es: v1alpha1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "an-es-name",
					Namespace: "default",
				},
				Spec: v1alpha1.ElasticsearchSpec{
					LicenseType: "gold",
				},
			}},
			want: "https://an-es-name-es.default.svc.cluster.local:9200",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExternalServiceURL(tt.args.es)
			assert.Equal(t, tt.want, got)
		})
	}
}
