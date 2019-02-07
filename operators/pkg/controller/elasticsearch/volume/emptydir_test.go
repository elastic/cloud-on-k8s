// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

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
