// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1

import (
	"testing"

	common "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1beta1"
)

func Test_noUnsupportedSettings(t *testing.T) {
	tests := []struct {
		name         string
		es           *Elasticsearch
		expectErrors bool
	}{

		{
			name:         "no settings OK",
			es:           es("7.0.0"),
			expectErrors: false,
		},
		{
			name: "warn of unsupported setting FAIL",
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []NodeSet{
						{
							Config: &common.Config{
								Data: map[string]interface{}{
									ClusterInitialMasterNodes: "foo",
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
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []NodeSet{
						{
							Config: &common.Config{
								Data: map[string]interface{}{
									ClusterInitialMasterNodes: "foo",
								},
							},
						},
						{
							Config: &common.Config{
								Data: map[string]interface{}{
									XPackSecurityTransportSslVerificationMode: "bar",
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
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []NodeSet{
						{
							Config: &common.Config{
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
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []NodeSet{
						{
							Config: &common.Config{
								Data: map[string]interface{}{
									XPackSecurityTransportSslCertificateAuthorities: "foo",
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
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []NodeSet{
						{
							Config: &common.Config{
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
