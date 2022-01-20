// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package version

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
)

func TestSupportedVersions(t *testing.T) {
	type args struct {
		v version.Version
	}
	tests := []struct {
		name        string
		args        args
		supported   []version.Version
		unsupported []version.Version
	}{
		{
			name: "6.x",
			args: args{
				v: version.MustParse("6.8.0"),
			},
			supported: []version.Version{
				version.MustParse("6.8.0"),
				version.MustParse("6.99.99"),
			},
			unsupported: []version.Version{
				version.MustParse("6.5.0"),
				version.MustParse("7.0.0"),
			},
		},
		{
			name: "7.x",
			args: args{
				v: version.MustParse("7.1.0"),
			},
			supported: []version.Version{
				version.MustParse("6.8.0"), // wire compat
				version.MustParse("7.2.0"),
				version.MustParse("7.99.99"),
			},
			unsupported: []version.Version{
				version.MustParse("6.6.0"),
				version.MustParse("8.0.0"),
			},
		},
		{
			name: "8.x",
			args: args{
				v: version.MustParse("8.0.0"),
			},
			supported: []version.Version{
				version.MustParse("7.17.0"),
				version.MustParse("8.9.0"),
			},
			unsupported: []version.Version{
				version.MustParse("7.1.0"), // supported by ECK but no direct upgrade path to 8.x
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs := SupportedVersions(tt.args.v)
			for _, v := range tt.supported {
				require.NoError(t, vs.WithinRange(v))
			}
			for _, v := range tt.unsupported {
				require.Error(t, vs.WithinRange(v))
			}
		})
	}
}

func Test_supportedVersionsWithMinimum(t *testing.T) {
	type args struct {
		v   version.Version
		min version.Version
	}
	tests := []struct {
		name string
		args args
		want *version.MinMaxVersion
	}{
		{
			name: "no minimum",
			args: args{
				v:   version.MustParse("7.10.0"),
				min: version.Version{},
			},
			want: &version.MinMaxVersion{
				Min: version.MustParse("6.8.0"),
				Max: version.MustParse("7.99.99"),
			},
		},
		{
			name: "v >= minimum",
			args: args{
				v:   version.MustParse("7.10.0"),
				min: version.MustParse("7.10.0"),
			},
			want: &version.MinMaxVersion{
				Min: version.MustParse("6.8.0"),
				Max: version.MustParse("7.99.99"),
			},
		},
		{
			name: "v < minimum",
			args: args{
				v:   version.MustParse("6.8.0"),
				min: version.MustParse("7.10.0"),
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := supportedVersionsWithMinimum(tt.args.v, tt.args.min); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("supportedVersionsWithMinimum() = %v, want %v", got, tt.want)
			}
		})
	}
}
