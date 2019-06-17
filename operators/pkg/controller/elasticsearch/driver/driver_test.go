// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/stretchr/testify/require"
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
				version.MustParse("6.7.0"),
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
				version.MustParse("6.7.0"), //wire compat
				version.MustParse("7.2.0"),
				version.MustParse("7.99.99"),
			},
			unsupported: []version.Version{
				version.MustParse("6.6.0"),
				version.MustParse("8.0.0"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs := SupportedVersions(tt.args.v)
			for _, v := range tt.supported {
				require.NoError(t, vs.Supports(v))
			}
			for _, v := range tt.unsupported {
				require.Error(t, vs.Supports(v))
			}
		})
	}
}
