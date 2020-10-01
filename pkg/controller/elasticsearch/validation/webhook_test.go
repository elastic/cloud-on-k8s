// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
)

func TestValidateElasticsearch(t *testing.T) {
	tests := []struct {
		name    string
		es      esv1.Elasticsearch
		wantErr bool
	}{
		{
			name: "has nodeset with invalid version",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "x.y",
					NodeSets: []esv1.NodeSet{
						{
							Config: &commonv1.Config{
								Data: map[string]interface{}{
									esv1.NodeMaster: "true",
									esv1.NodeData:   "true",
									esv1.NodeIngest: "false",
									esv1.NodeML:     "false",
								},
							},
							Count: 1,
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateElasticsearch(tt.es); (err != nil) != tt.wantErr {
				t.Errorf("Elasticsearch.ValidateCreate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

