// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build integration

package fs

import (
	"context"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewFileWatcher(t *testing.T) {

	requireEventEquals := func(c chan []string, expected []string, timeout time.Duration) {
		select {
		case event := <-c:
			require.Equal(t, expected, event)
		case <-time.After(timeout):
			require.Fail(t, "no event observed")
		}
	}

	requireNoEvent := func(c chan []string, timeout time.Duration) {
		select {
		case event := <-c:
			require.Fail(t, "no event expected", "event: %v", event)
		case <-time.After(timeout):
		}
	}

	dir, err := ioutil.TempDir("", "file-watcher-test")
	require.NoError(t, err)

	defer os.RemoveAll(dir)
	file1 := filepath.Join(dir, "file1")
	file2 := filepath.Join(dir, "file2")
	file3 := filepath.Join(dir, "file3")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan []string)

	onChange := func(paths []string) {
		events <- paths
	}

	// file 2 exists before is setup
	require.NoError(t, ioutil.WriteFile(file2, []byte("contents"), 0644))

	// setup watcher
	watcher := NewFileWatcher(ctx, []string{file1, file2, file3}, onChange, 1*time.Millisecond)

	// file 1 and 3 is only created afterwards
	require.NoError(t, ioutil.WriteFile(file1, []byte("contents"), 0644))
	require.NoError(t, ioutil.WriteFile(file3, []byte("contents"), 0644))

	// we start watcher only now to be able to simulate multiple events happening in one observation interval
	// which would otherwise lead to a flaky test
	go watcher.Run()

	requireEventEquals(events, []string{file1, file3}, 1*time.Second)

	// update file 2
	require.NoError(t, ioutil.WriteFile(file2, []byte("new contents"), 0644))
	requireEventEquals(events, []string{file2}, 1*time.Second)

	// remove file1
	require.NoError(t, os.Remove(file1))
	requireEventEquals(events, []string{file1}, 1*time.Second)

	// files that are not watched should not trigger events
	notWatched := filepath.Join(dir, "not-watched")
	require.NoError(t, ioutil.WriteFile(notWatched, []byte("contents"), 0644))
	requireNoEvent(events, 100*time.Millisecond)

}
