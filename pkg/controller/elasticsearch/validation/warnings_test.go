// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
)

func Test_noUnsupportedSettings(t *testing.T) {
	tests := []struct {
		name         string
		es           esv1.Elasticsearch
		expectErrors bool
	}{

		{
			name:         "no settings OK",
			es:           es("7.0.0"),
			expectErrors: false,
		},
		{
			name: "warn of unsupported setting FAIL",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []esv1.NodeSet{
						{
							Config: &commonv1.Config{
								Data: map[string]interface{}{
									esv1.ClusterInitialMasterNodes: "foo",
								},
							},
							Count: 1,
						},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "warn of unsupported in multiple nodes FAIL",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []esv1.NodeSet{
						{
							Config: &commonv1.Config{
								Data: map[string]interface{}{
									esv1.ClusterInitialMasterNodes: "foo",
								},
							},
						},
						{
							Config: &commonv1.Config{
								Data: map[string]interface{}{
									esv1.XPackSecurityTransportSslVerificationMode: "bar",
								},
							},
						},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "non unsupported setting OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []esv1.NodeSet{
						{
							Config: &commonv1.Config{
								Data: map[string]interface{}{
									"node.attr.box_type": "foo",
								},
							},
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "supported settings with unsupported string prefix OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []esv1.NodeSet{
						{
							Config: &commonv1.Config{
								Data: map[string]interface{}{
									esv1.XPackSecurityTransportSslCertificateAuthorities: "foo",
								},
							},
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "settings are canonicalized before validation",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []esv1.NodeSet{
						{
							Config: &commonv1.Config{
								Data: map[string]interface{}{
									"cluster": map[string]interface{}{
										"initial_master_nodes": []string{"foo", "bar"},
									},
									"node.attr.box_type": "foo",
								},
							},
						},
					},
				},
			},
			expectErrors: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := noUnsupportedSettings(tt.es)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed noUnsupportedSettings(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.es.Spec.Version)
			}
		})
	}
}
