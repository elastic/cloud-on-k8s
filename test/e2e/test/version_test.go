// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
)

func TestIsValidUpgrade(t *testing.T) {
	tests := []struct {
		from    string
		to      string
		isValid bool
	}{
		// valid upgrade paths
		{from: "7.1.1", to: "7.6.0", isValid: true},
		{from: "7.17.0", to: "8.0.0", isValid: true},
		{from: "8.0.0", to: "8.15.0", isValid: true},
		// invalid upgrade paths
		{from: "7.16.0", to: "8.0.0", isValid: false},
		{from: "7.6.0", to: "8.0.0-SNAPSHOT", isValid: false},
		{from: "7.6.0", to: "7.6.0", isValid: false},
		{from: "7.6.0", to: "7.5.0", isValid: false},
		{from: "7.6.1", to: "7.6.0", isValid: false},
		{from: "8.0.0", to: "7.17.0", isValid: false},
		{from: "7.6.0", to: "9.0.0", isValid: false},
		{from: "7.6.0-SNAPSHOT", to: "7.7.0", isValid: false},
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

func TestGetUpgradePathTo8x(t *testing.T) {
	tests := []struct {
		current string
		src     string
		dst     string
	}{
		{current: "7.17.0", src: "7.17.0", dst: LatestReleasedVersion8x},
		{current: "8.0.0", src: "8.0.0", dst: LatestReleasedVersion8x},
		{current: "8.99.0-SNAPSHOT", src: LatestReleasedVersion8x, dst: "8.99.0-SNAPSHOT"},
	}

	for _, tt := range tests {
		src, dst := GetUpgradePathTo8x(tt.current)
		require.Equal(t, tt.src, src)
		require.Equal(t, tt.dst, dst)
	}
}

func TestIsVersionSupportedForWolfi(t *testing.T) {
	tests := []struct {
		version   string
		supported bool
	}{
		// 7.x: no Wolfi variants
		{version: "7.17.29", supported: false},
		// 8.x below minimum: no Wolfi variants
		{version: "8.0.0", supported: false},
		{version: "8.15.9", supported: false},
		// 8.16.0-8.16.3: Logstash openssl missing; 8.16.4+ OK
		{version: "8.16.0", supported: false},
		{version: "8.16.3", supported: false},
		{version: "8.16.4", supported: true},
		{version: "8.16.5", supported: true},
		// 8.17.0-8.17.1: Logstash openssl missing; 8.17.2+ OK
		{version: "8.17.0", supported: false},
		{version: "8.17.1", supported: false},
		{version: "8.17.2", supported: true},
		{version: "8.17.3", supported: true},
		{version: "8.18.0", supported: true},
		{version: "8.19.20", supported: true},
		// 9.x: Wolfi variants available
		{version: "9.0.0", supported: true},
		{version: "9.5.1", supported: true},
		{version: "9.6.0-SNAPSHOT", supported: true},
	}
	for _, tt := range tests {
		ver := version.MustParse(tt.version)
		supported, _ := isVersionSupportedForWolfi(ver)
		require.Equal(t, tt.supported, supported, "version %s", tt.version)
	}
}
