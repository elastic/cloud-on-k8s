// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import "testing"

func TestConfig_is(t *testing.T) {
	type args struct {
		key string
	}
	tests := []struct {
		name string
		c    Config
		args args
		want bool
	}{
		{
			name: "default to false",
			c:    nil,
			args: args{
				key: NodeMaster,
			},
			want: false,
		},
		{
			name: "detect true",
			c: Config{
				NodeMaster: "true",
			},
			args: args{
				key: NodeMaster,
			},
			want: true,
		},
		{
			name: "ignore garbage",
			c: Config{
				NodeMaster: "lskdfj",
			},
			args: args{
				key: NodeMaster,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.is(tt.args.key); got != tt.want {
				t.Errorf("Config.is() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_EqualRoles(t *testing.T) {
	type args struct {
		c2 Config
	}
	tests := []struct {
		name string
		c    Config
		args args
		want bool
	}{
		{
			name: "empty is equal",
			c:    nil,
			args: args{},
			want: true,
		},
		{
			name: "same is equal",
			c: Config{
				NodeMaster: "true",
			},
			args: args{
				c2: Config{NodeMaster: "true"},
			},
			want: true,
		},
		{
			name: "detect differences",
			c: Config{
				NodeMaster: "true",
				NodeData:   "true",
			},
			args: args{
				c2: Config{NodeData: "true"},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.EqualRoles(tt.args.c2); got != tt.want {
				t.Errorf("Config.EqualRoles() = %v, want %v", got, tt.want)
			}
		})
	}
}
