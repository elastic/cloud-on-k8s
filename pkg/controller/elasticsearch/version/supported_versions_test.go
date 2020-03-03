// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
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
				version.MustParse("6.8.0"), //wire compat
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
				version.MustParse("7.4.0"),
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
				require.NoError(t, vs.Supports(v))
			}
			for _, v := range tt.unsupported {
				require.Error(t, vs.Supports(v))
			}
		})
	}
}

func TestSupports(t *testing.T) {
	tests := []struct {
		name        string
		lhsv        LowestHighestSupportedVersions
		ver         version.Version
		expectError bool
	}{
		{
			name: "in range",
			lhsv: LowestHighestSupportedVersions{
				LowestSupportedVersion:  version.MustParse("2.0.1"),
				HighestSupportedVersion: version.MustParse("3.0.0"),
			},
			ver:         version.MustParse("2.0.2-rc0"),
			expectError: false,
		},
		{
			name: "out of range",
			lhsv: LowestHighestSupportedVersions{
				LowestSupportedVersion:  version.MustParse("2.0.1"),
				HighestSupportedVersion: version.MustParse("3.0.0"),
			},
			ver:         version.MustParse("3.0.1"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.lhsv.Supports(tt.ver)
			actual := err != nil
			if tt.expectError != actual {
				t.Errorf("failed Supports(). Name: %v, actual: %v, wanted: %v, value: %v", tt.name, err, tt.expectError, tt.ver)
			}
		})
	}
}
