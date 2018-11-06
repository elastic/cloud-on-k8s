package elasticsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEmptyDirDefault(t *testing.T) {
	v := NewDefaultEmptyDirVolume()
	assert.Equal(t, v.Volume().Name, "volume")
	assert.Equal(t, v.VolumeMount().Name, "volume")
	assert.Equal(t, v.VolumeMount().MountPath, "/volume")
	assert.Equal(t, v.DataPath(), "/volume/data")
	assert.Equal(t, v.LogsPath(), "/volume/logs")
}
