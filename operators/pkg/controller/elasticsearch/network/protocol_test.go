// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package network

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
)

func TestProtocolForCluster(t *testing.T) {
	tests := []struct {
		name string
		es   v1alpha1.Elasticsearch
		want string
	}{
		{
			name: "basic license: http",
			es: v1alpha1.Elasticsearch{
				Spec: v1alpha1.ElasticsearchSpec{
					LicenseType: "basic",
				},
			},
			want: "http",
		},
		{
			name: "trial license: https",
			es: v1alpha1.Elasticsearch{
				Spec: v1alpha1.ElasticsearchSpec{
					LicenseType: "trial",
				},
			},
			want: "https",
		},
		{
			name: "gold license: https",
			es: v1alpha1.Elasticsearch{
				Spec: v1alpha1.ElasticsearchSpec{
					LicenseType: "gold",
				},
			},
			want: "https",
		},
		{
			name: "platinum license: https",
			es: v1alpha1.Elasticsearch{
				Spec: v1alpha1.ElasticsearchSpec{
					LicenseType: "platinum",
				},
			},
			want: "https",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProtocolForCluster(tt.es); got != tt.want {
				t.Errorf("ProtocolForCluster() = %v, want %v", got, tt.want)
			}
		})
	}
}
