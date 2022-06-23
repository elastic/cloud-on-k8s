// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1

import (
	"testing"

	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1beta1"
)

func TestConfig_RoleDefaults(t *testing.T) {
	type args struct {
		c2 v1beta1.Config
	}
	tests := []struct {
		name string
		c    v1beta1.Config
		args args
		want bool
	}{
		{
			name: "empty is equal",
			c:    v1beta1.Config{},
			args: args{},
			want: true,
		},
		{
			name: "same is equal",
			c: v1beta1.Config{
				Data: map[string]interface{}{
					NodeMaster: true,
				},
			},
			args: args{
				c2: v1beta1.Config{
					Data: map[string]interface{}{
						NodeMaster: true,
					},
				},
			},
			want: true,
		},
		{
			name: "detect differences",
			c: v1beta1.Config{
				Data: map[string]interface{}{
					NodeMaster: false,
					NodeData:   true,
				},
			},
			args: args{
				c2: v1beta1.Config{
					Data: map[string]interface{}{
						NodeData: true,
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c1, err := UnpackConfig(&tt.c)
			require.NoError(t, err)
			c2, err := UnpackConfig(&tt.args.c2)
			require.NoError(t, err)
			if got := c1.Node == c2.Node; got != tt.want {
				t.Errorf("Config.EqualRoles() = %v, want %v", got, tt.want)
			}
		})
	}
}

var testFixture = v1beta1.Config{
	Data: map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": 1.0,
			},
			"d": 1,
		},
		"a.b.foo": "bar",
		"e":       []interface{}{1, 2, 3},
		"f":       true,
	},
}

var expectedJSONized = v1beta1.Config{
	Data: map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": 1.0,
			},
			"d": float64(1),
		},
		"a.b.foo": "bar",
		"e":       []interface{}{float64(1), float64(2), float64(3)},
		"f":       true,
	},
}

func TestConfig_DeepCopyInto(t *testing.T) {
	tests := []struct {
		name     string
		in       v1beta1.Config
		expected v1beta1.Config
	}{
		{
			name:     "deep copy via JSON roundtrip changes some types",
			in:       testFixture,
			expected: expectedJSONized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out v1beta1.Config
			tt.in.DeepCopyInto(&out)
			if diff := deep.Equal(out, tt.expected); diff != nil {
				t.Error(diff)
			}
		})
	}
}

func TestConfig_DeepCopy(t *testing.T) {
	tests := []struct {
		name string
		in   v1beta1.Config
		want v1beta1.Config
	}{
		{
			name: "deep copy via JSON roundtrip changes some types",
			in:   testFixture,
			want: expectedJSONized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := deep.Equal(tt.in.DeepCopy(), &tt.want); diff != nil {
				t.Error(diff)
			}
		})
	}
}

func TestConfig_Unpack(t *testing.T) {
	tests := []struct {
		name    string
		args    *v1beta1.Config
		want    ElasticsearchSettings
		wantErr bool
	}{
		{
			name: "happy path",
			args: &v1beta1.Config{
				Data: map[string]interface{}{
					"node": map[string]interface{}{
						"master": false,
						"data":   true,
					},
					"cluster": map[string]interface{}{
						"initial_master_nodes": []string{"a", "b"},
					},
				},
			},
			want: ElasticsearchSettings{
				Node: Node{
					Master: false,
					Data:   true,
					Ingest: true,
					ML:     true,
				},
				Cluster: ClusterSettings{
					InitialMasterNodes: []string{"a", "b"},
				},
			},
			wantErr: false,
		},
		{
			name:    "Unpack is nil safe",
			args:    nil,
			want:    DefaultCfg,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UnpackConfig(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Unpack() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := deep.Equal(tt.want, got); diff != nil {
				t.Error(diff)
			}
		})
	}
}
