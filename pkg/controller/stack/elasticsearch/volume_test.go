package elasticsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestEmptyDirDefault(t *testing.T) {
	v := NewDefaultEmptyDirVolume()
	assert.Equal(t, v.Volume().Name, "volume")
	assert.Equal(t, v.VolumeMount().Name, "volume")
	assert.Equal(t, v.VolumeMount().MountPath, "/volume")
	assert.Equal(t, v.DataPath(), "/volume/data")
	assert.Equal(t, v.LogsPath(), "/volume/logs")
}

func TestSecretVolumeItemProjection(t *testing.T) {

	testVolume := NewSecretVolume("secret", "secrets")
	testVolume.items = []string{"foo"}

	tests := []struct {
		volume   SecretVolume
		expected []corev1.KeyToPath
	}{
		{
			volume:   NewSecretVolume("secret", "/secrets"),
			expected: nil,
		},
		{
			volume: testVolume,
			expected: []corev1.KeyToPath{
				corev1.KeyToPath{
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
