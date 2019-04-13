// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/validation"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/name"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_hasMaster(t *testing.T) {
	failedValidation := validation.Result{Allowed: false, Reason: masterRequiredMsg}
	type args struct {
		esCluster v1alpha1.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want validation.Result
	}{
		{
			name: "no topology",
			args: args{
				esCluster: *es("6.7.0"),
			},
			want: failedValidation,
		},
		{
			name: "topology but no master",
			args: args{
				esCluster: v1alpha1.Elasticsearch{
					Spec: v1alpha1.ElasticsearchSpec{
						Version: "7.0.0",
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
						Version: "7.0.0",
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
						Version: "7.0.0",
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
			want: validation.Result{Allowed: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewValidationContext(nil, tt.args.esCluster)
			require.NoError(t, err)
			if got := hasMaster(*ctx); got != tt.want {
				t.Errorf("hasMaster() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_supportedVersion(t *testing.T) {
	type args struct {
		esCluster estype.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want validation.Result
	}{
		{
			name: "unsupported FAIL",
			args: args{
				esCluster: *es("1.0.0"),
			},
			want: validation.Result{Allowed: false, Reason: unsupportedVersion(&version.Version{
				Major: 1,
				Minor: 0,
				Patch: 0,
				Label: "",
			})},
		},
		{
			name: "supported OK",
			args: args{
				esCluster: *es("6.7.0"),
			},
			want: validation.OK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewValidationContext(nil, tt.args.esCluster)
			require.NoError(t, err)
			if got := supportedVersion(*ctx); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("supportedVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_nameLength(t *testing.T) {
	type args struct {
		esCluster estype.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want validation.Result
	}{
		{
			name: "name length too long",
			args: args{
				esCluster: estype.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "that-is-a-very-long-name-with-37chars",
					},
					Spec: estype.ElasticsearchSpec{Version: "6.7.0"},
				},
			},
			want: validation.Result{Allowed: false, Reason: fmt.Sprintf(nameTooLongErrMsg, name.MaxElasticsearchNameLength)},
		},
		{
			name: "name length OK",
			args: args{
				esCluster: estype.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "that-is-a-very-long-name-with-36char",
					},
					Spec: estype.ElasticsearchSpec{Version: "6.7.0"},
				},
			},
			want: validation.OK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewValidationContext(nil, tt.args.esCluster)
			require.NoError(t, err)
			if got := nameLength(*ctx); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("supportedVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}
