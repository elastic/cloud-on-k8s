package volume

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewEmptyDirVolume(t *testing.T) {
	v := NewEmptyDirVolume("name", "/mountPath")
	assert.Equal(t, v.Volume().Name, "name")
	assert.Equal(t, v.VolumeMount().Name, "name")
	assert.Equal(t, v.VolumeMount().MountPath, "/mountPath")
}
