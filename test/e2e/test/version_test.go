// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"testing"

	"github.com/stretchr/testify/require"
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
		{from: "7.17.0", to: "8.0.0", isValid: true},
		// invalid upgrade paths
		{from: "7.16.0", to: "8.0.0", isValid: false},
		{from: "7.6.0", to: "8.0.0-SNAPSHOT", isValid: false},
		{from: "7.6.0", to: "7.6.0", isValid: false},
		{from: "7.6.0", to: "7.5.0", isValid: false},
		{from: "7.6.1", to: "7.6.0", isValid: false},
		{from: "7.6.0", to: "6.8.5", isValid: false},
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
