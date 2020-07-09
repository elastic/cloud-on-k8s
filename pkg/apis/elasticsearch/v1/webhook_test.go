// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

func TestElasticsearch_ValidateCreate(t *testing.T) {
	tests := []struct {
		name    string
		es      *Elasticsearch
		wantErr bool
	}{
		{
			name: "has nodeset with invalid version",
			es: &Elasticsearch{
				Spec: ElasticsearchSpec{
					Version: "x.y",
					NodeSets: []NodeSet{
						{
							Config: &commonv1.Config{
								Data: map[string]interface{}{
									NodeMaster: "true",
									NodeData:   "true",
									NodeIngest: "false",
									NodeML:     "false",
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
			if err := tt.es.ValidateCreate(); (err != nil) != tt.wantErr {
				t.Errorf("Elasticsearch.ValidateCreate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
