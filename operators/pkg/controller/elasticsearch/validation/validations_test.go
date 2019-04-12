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
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_hasMaster(t *testing.T) {
	failedValidation := Result{Allowed: false, Reason: masterRequiredMsg}
	type args struct {
		esCluster v1alpha1.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want Result
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
						Nodes: []v1alpha1.NodeSpec{
							{
								Config: v1alpha1.Config{
									Data: map[string]interface{}{
										v1alpha1.NodeMaster: "false",
										v1alpha1.NodeData:   "false",
										v1alpha1.NodeIngest: "false",
										v1alpha1.NodeML:     "false",
									},
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
						Nodes: []v1alpha1.NodeSpec{
							{
								Config: v1alpha1.Config{
									Data: map[string]interface{}{
										v1alpha1.NodeMaster: "true",
										v1alpha1.NodeData:   "false",
										v1alpha1.NodeIngest: "false",
										v1alpha1.NodeML:     "false",
									},
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
						Nodes: []v1alpha1.NodeSpec{
							{
								Config: v1alpha1.Config{
									Data: map[string]interface{}{
										v1alpha1.NodeMaster: "true",
										v1alpha1.NodeData:   "false",
										v1alpha1.NodeIngest: "false",
										v1alpha1.NodeML:     "false",
									},
								},
								NodeCount: 1,
							},
						},
					},
				},
			},
			want: Result{Allowed: true},
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
		want Result
	}{
		{
			name: "unsupported FAIL",
			args: args{
				esCluster: *es("1.0.0"),
			},
			want: Result{Allowed: false, Reason: unsupportedVersion(&version.Version{
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
			want: OK,
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

func Test_noBlacklistedSettings(t *testing.T) {
	type args struct {
		es estype.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want Result
	}{
		{
			name: "no settings OK",
			args: args{
				es: *es("7.0.0"),
			},
			want: OK,
		},
		{
			name: "enforce blacklist FAIL",
			args: args{
				es: estype.Elasticsearch{
					Spec: estype.ElasticsearchSpec{
						Version: "7.0.0",
						Nodes: []estype.NodeSpec{
							{
								Config: estype.Config{
									Data: map[string]interface{}{
										settings.ClusterInitialMasterNodes: "foo",
									},
								},
								NodeCount: 1,
							},
						},
					},
				},
			},
			want: Result{Allowed: false, Reason: "node[0]: cluster.initial_master_nodes is not user configurable"},
		},
		{
			name: "enforce blacklist in multiple nodes FAIL",
			args: args{
				es: estype.Elasticsearch{
					Spec: estype.ElasticsearchSpec{
						Version: "7.0.0",
						Nodes: []estype.NodeSpec{
							{
								Config: estype.Config{
									Data: map[string]interface{}{
										settings.ClusterInitialMasterNodes: "foo",
									},
								},
							},
							{
								Config: estype.Config{
									Data: map[string]interface{}{
										settings.XPackSecurityTransportSslVerificationMode: "bar",
									},
								},
							},
						},
					},
				},
			},
			want: Result{
				Allowed: false,
				Reason:  "node[0]: cluster.initial_master_nodes; node[1]: xpack.security.transport.ssl.verification_mode is not user configurable",
			},
		},
		{
			name: "non blacklisted setting OK",
			args: args{
				es: estype.Elasticsearch{
					Spec: estype.ElasticsearchSpec{
						Version: "7.0.0",
						Nodes: []estype.NodeSpec{
							{
								Config: estype.Config{
									Data: map[string]interface{}{
										"node.attr.box_type": "foo",
									},
								},
							},
						},
					},
				},
			},
			want: OK,
		},

		{
			name: "settings are canonicalized before validation",
			args: args{
				es: estype.Elasticsearch{
					Spec: estype.ElasticsearchSpec{
						Version: "7.0.0",
						Nodes: []estype.NodeSpec{
							{
								Config: estype.Config{
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
			},
			want: Result{Allowed: false, Reason: "node[0]: cluster.initial_master_nodes is not user configurable"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewValidationContext(nil, tt.args.es)
			require.NoError(t, err)
			if got := noBlacklistedSettings(*ctx); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("noBlacklistedSettings() = %v, want %v", got, tt.want)
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
		want Result
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
			want: Result{Allowed: false, Reason: fmt.Sprintf(nameTooLongErrMsg, name.MaxElasticsearchNameLength)},
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
			want: OK,
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
