// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package volume

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestSecretVolumeItemProjection(t *testing.T) {

	testVolume := NewSelectiveSecretVolumeWithMountPath("secret", "secrets", "/mnt", []string{"foo"})
	tests := []struct {
		volume   SecretVolume
		expected []corev1.KeyToPath
	}{
		{
			volume:   NewSecretVolumeWithMountPath("secret", "secrets", "/secrets"),
			expected: nil,
		},
		{
			volume: testVolume,
			expected: []corev1.KeyToPath{
				{
					Key:  "foo",
					Path: "foo",
				},
			},
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.volume.Volume().Secret.Items)
	}
}
