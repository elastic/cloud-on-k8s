// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package waitforannotations

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnnotationsPresent(t *testing.T) {
	write := func(t *testing.T, content string) string {
		t.Helper()
		f := filepath.Join(t.TempDir(), "annotations")
		require.NoError(t, os.WriteFile(f, []byte(content), 0o600))
		return f
	}

	t.Run("all keys present with quoted values", func(t *testing.T) {
		path := write(t, `topology.kubernetes.io/zone="us-east-1a"
topology.kubernetes.io/region="us-east-1"
`)
		ok, err := annotationsPresent(path, []string{"topology.kubernetes.io/zone", "topology.kubernetes.io/region"})
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("one key missing", func(t *testing.T) {
		path := write(t, `topology.kubernetes.io/zone="us-east-1a"
`)
		ok, err := annotationsPresent(path, []string{"topology.kubernetes.io/zone", "topology.kubernetes.io/region"})
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("key whose value contains equals sign", func(t *testing.T) {
		path := write(t, `my-annotation="base64=="`+"\n")
		ok, err := annotationsPresent(path, []string{"my-annotation"})
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("file does not exist returns error", func(t *testing.T) {
		_, err := annotationsPresent(filepath.Join(t.TempDir(), "no-such-file"), []string{"k"})
		require.Error(t, err)
	})

	t.Run("empty file returns false", func(t *testing.T) {
		path := write(t, "")
		ok, err := annotationsPresent(path, []string{"topology.kubernetes.io/zone"})
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("line without equals is skipped", func(t *testing.T) {
		path := write(t, "not-a-key-value-pair\ntopology.kubernetes.io/zone=\"us-east-1a\"\n")
		ok, err := annotationsPresent(path, []string{"topology.kubernetes.io/zone"})
		require.NoError(t, err)
		assert.True(t, ok)
	})
}

// setViperFlags configures viper with the given values for doRun and resets it after the test.
func setViperFlags(t *testing.T, file string, annotations []string, timeout, pollInterval time.Duration) {
	t.Helper()
	t.Cleanup(viper.Reset)
	viper.Set(FileFlag, file)
	viper.Set(AnnotationFlag, annotations)
	viper.Set(TimeoutFlag, timeout)
	viper.Set(PollIntervalFlag, pollInterval)
}

func TestDoRun_Timeout(t *testing.T) {
	f := filepath.Join(t.TempDir(), "annotations")
	require.NoError(t, os.WriteFile(f, []byte(`other.key="value"`+"\n"), 0o600))

	setViperFlags(t, f, []string{"topology.kubernetes.io/zone"}, 50*time.Millisecond, 10*time.Millisecond)

	err := doRun(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestDoRun_Success(t *testing.T) {
	f := filepath.Join(t.TempDir(), "annotations")
	require.NoError(t, os.WriteFile(f, []byte(`topology.kubernetes.io/zone="us-east-1a"`+"\n"), 0o600))

	setViperFlags(t, f, []string{"topology.kubernetes.io/zone"}, 0, 10*time.Millisecond)

	err := doRun(context.Background())
	require.NoError(t, err)
}

func TestDoRun_MissingFile(t *testing.T) {
	setViperFlags(t, "", []string{"topology.kubernetes.io/zone"}, 0, 10*time.Millisecond)

	err := doRun(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), FileFlag)
}

func TestDoRun_MissingAnnotation(t *testing.T) {
	f := filepath.Join(t.TempDir(), "annotations")
	setViperFlags(t, f, nil, 0, 10*time.Millisecond)

	err := doRun(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), AnnotationFlag)
}
