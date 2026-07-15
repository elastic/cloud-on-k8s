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

const testOperatorImage = "docker.elastic.co/eck/eck-operator:test"

func TestWaitForAnnotationsInitContainer(t *testing.T) {
	t.Run("builds container with operator image and subcommand", func(t *testing.T) {
		c, err := WaitForAnnotationsInitContainer(testOperatorImage, []string{"topology.kubernetes.io/zone", "topology.kubernetes.io/region"})
		require.NoError(t, err)

		assert.Equal(t, WaitForAnnotationsContainerName, c.Name)
		assert.Equal(t, testOperatorImage, c.Image)

		require.Len(t, c.VolumeMounts, 1)
		assert.Equal(t, "downward-api", c.VolumeMounts[0].Name)
		assert.Equal(t, "/mnt/elastic-internal/downward-api", c.VolumeMounts[0].MountPath)
		assert.True(t, c.VolumeMounts[0].ReadOnly)

		// Command must be the operator subcommand invocation, not a shell script.
		require.GreaterOrEqual(t, len(c.Command), 4)
		assert.Equal(t, "/elastic-operator", c.Command[0])
		assert.Equal(t, "wait-for-annotations", c.Command[1])
		assert.Equal(t, "--file=/mnt/elastic-internal/downward-api/annotations", c.Command[2])
		assert.Contains(t, strings.Join(c.Command, " "), "--annotation=topology.kubernetes.io/zone")
		assert.Contains(t, strings.Join(c.Command, " "), "--annotation=topology.kubernetes.io/region")

		// Resources and Env are left unset so they are inherited via WithInitContainerDefaults.
		assert.Empty(t, c.Resources.Limits)
		assert.Empty(t, c.Resources.Requests)
		assert.Empty(t, c.Env)
	})

	t.Run("error when operator image is empty", func(t *testing.T) {
		_, err := WaitForAnnotationsInitContainer("", []string{"topology.kubernetes.io/zone"})
		require.Error(t, err)
	})
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
