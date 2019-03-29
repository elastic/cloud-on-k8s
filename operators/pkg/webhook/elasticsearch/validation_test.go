// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
)

func Test_hasMaster(t *testing.T) {
	failedValidation := ValidationResult{Allowed: false, Reason: masterRequiredMsg}
	type args struct {
		esCluster v1alpha1.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want ValidationResult
	}{
		{
			name: "no topology",
			args: args{
				esCluster: v1alpha1.Elasticsearch{},
			},
			want: failedValidation,
		},
		{
			name: "topology but no master",
			args: args{
				esCluster: v1alpha1.Elasticsearch{
					Spec: v1alpha1.ElasticsearchSpec{
						Topology: []v1alpha1.TopologyElementSpec{
							{
								NodeTypes: v1alpha1.NodeTypesSpec{
									Master: false,
									Data:   false,
									Ingest: false,
									ML:     false,
								},
							},
						},
					},
				},
			},
			want: failedValidation,
		},
		{
			name: "master but zero sized",
			args: args{
				esCluster: v1alpha1.Elasticsearch{
					Spec: v1alpha1.ElasticsearchSpec{
						Topology: []v1alpha1.TopologyElementSpec{
							{
								NodeTypes: v1alpha1.NodeTypesSpec{
									Master: true,
									Data:   false,
									Ingest: false,
									ML:     false,
								},
							},
						},
					},
				},
			},
			want: failedValidation,
		},
		{
			name: "has master",
			args: args{
				esCluster: v1alpha1.Elasticsearch{
					Spec: v1alpha1.ElasticsearchSpec{
						Topology: []v1alpha1.TopologyElementSpec{
							{
								NodeTypes: v1alpha1.NodeTypesSpec{
									Master: true,
									Data:   false,
									Ingest: false,
									ML:     false,
								},
								NodeCount: 1,
							},
						},
					},
				},
			},
			want: ValidationResult{Allowed: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasMaster(nil, &tt.args.esCluster); got != tt.want {
				t.Errorf("hasMaster() = %v, want %v", got, tt.want)
			}
		})
	}
}
