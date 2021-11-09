// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
)

func TestIsValidUpgrade(t *testing.T) {
	tests := []struct {
		from    string
		to      string
		isValid bool
	}{
		// valid upgrade paths
		{from: "6.8.5", to: "6.8.6", isValid: true},
		{from: "6.8.5", to: "7.1.1", isValid: true},
		{from: "7.1.1", to: "7.6.0", isValid: true},
		{from: "7.6.0", to: "8.0.0", isValid: true},
		{from: "7.6.0", to: "8.0.0-SNAPSHOT", isValid: true},
		// invalid upgrade paths
		{from: "7.6.0", to: "7.6.0", isValid: false},
		{from: "7.6.0", to: "7.5.0", isValid: false},
		{from: "7.6.1", to: "7.6.0", isValid: false},
		{from: "7.6.0", to: "6.8.5", isValid: false},
		{from: "7.6.0", to: "9.0.0", isValid: false},
	}

	for _, tt := range tests {
		isValid, err := isValidUpgrade(tt.from, tt.to)
		require.NoError(t, err)
		if tt.isValid != isValid {
			t.Errorf(`isValidUpgrade("%s", "%s") = %v, want %v`, tt.from, tt.to, isValid, tt.isValid)
		}
		require.Equal(t, tt.isValid, isValid)
	}
}

func Test_isSnapshot(t *testing.T) {
	type args struct {
		ver version.Version
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "stable",
			args: args{
				ver: version.MustParse("7.13.0"),
			},
			want: false,
		},
		{
			name: "pre-release",
			args: args{
				ver: version.MustParse("7.13.0-SNAPSHOT"),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSnapshotVersion(tt.args.ver); got != tt.want {
				t.Errorf("isSnapshot() = %v, want %v", got, tt.want)
			}
		})
	}
}
