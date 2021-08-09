// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

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
			name: "7.15.0-SNAPSHOT is supported",
			args: args{
				v: version.MustParse("7.15.0-SNAPSHOT"),
			},
			want: true,
		},
		{
			name: "7.15.0 is supported",
			args: args{
				v: version.MustParse("7.15.0"),
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
			if got := supportsNodeshutdown(tt.args.v); got != tt.want {
				t.Errorf("supportsNodeshutdown() = %v, want %v", got, tt.want)
			}
		})
	}
}
