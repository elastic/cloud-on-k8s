// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
)

func Test_supportsNodeshutdown(t *testing.T) {
	type args struct {
		v version.Version
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "7.15.2 is supported",
			args: args{
				v: version.MustParse("7.15.2"),
			},
			want: true,
		},
		{
			name: "7.15.3-SNAPSHOT is supported",
			args: args{
				v: version.MustParse("7.15.3-SNAPSHOT"),
			},
			want: true,
		},
		{
			name: "7.14.0 is not",
			args: args{
				v: version.MustParse("7.14.0"),
			},
			want: false,
		},
		{
			name: "8.0.0 will be supported",
			args: args{
				v: version.MustParse("8.0.0"),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := supportsNodeShutdown(tt.args.v); got != tt.want {
				t.Errorf("supportsNodeShutdown() = %v, want %v", got, tt.want)
			}
		})
	}
}
