// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodelabels

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitForAnnotationsInitContainer(t *testing.T) {
	c, err := WaitForAnnotationsInitContainer([]string{"topology.kubernetes.io/zone", "topology.kubernetes.io/region"})
	require.NoError(t, err)
	assert.Equal(t, WaitForAnnotationsContainerName, c.Name)
	require.Len(t, c.VolumeMounts, 1)
	assert.Equal(t, "downward-api", c.VolumeMounts[0].Name)
	assert.Equal(t, "/mnt/elastic-internal/downward-api", c.VolumeMounts[0].MountPath)
	assert.True(t, c.VolumeMounts[0].ReadOnly)

	require.Len(t, c.Command, 3)
	assert.Equal(t, "bash", c.Command[0])
	assert.Equal(t, "-c", c.Command[1])
	script := c.Command[2]
	assert.Contains(t, script, "topology.kubernetes.io/zone topology.kubernetes.io/region")
	assert.Contains(t, script, "/mnt/elastic-internal/downward-api/annotations")
	// Each expected annotation is matched at the beginning of a line followed by '=':
	assert.Contains(t, script, `grep -qE "^${expected_annotation}="`)
	// No image/resources are set so they are inherited from the main container.
	assert.Equal(t, "", c.Image)
	assert.Empty(t, c.Resources.Limits)
	assert.Empty(t, c.Resources.Requests)
}

func TestDownwardAPIVolume_IncludesAnnotations(t *testing.T) {
	v := DownwardAPIVolume().Volume()
	require.NotNil(t, v.VolumeSource.DownwardAPI)
	paths := make([]string, 0, len(v.VolumeSource.DownwardAPI.Items))
	for _, item := range v.VolumeSource.DownwardAPI.Items {
		paths = append(paths, item.Path)
	}
	assert.Contains(t, strings.Join(paths, ","), "annotations")
}
