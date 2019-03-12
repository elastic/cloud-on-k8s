// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package fs

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func expectEvents(t *testing.T, events chan struct{}, min int, max int, during time.Duration) {
	timeout := time.After(during)
	got := 0
	for {
		select {
		case <-events:
			got++
		case <-timeout:
			if got < min || got > max {
				t.Errorf("got %d out instead of  [%d - %d] events after timeout", got, min, max)
			}
			return
		}
	}
}

func Test_FileWatcher(t *testing.T) {
	// Test that the file watcher behaves as expected in the common case.
	// Mostly checking everything is correctly plugged together.
	// Specific corner cases and detailed behaviours are tested
	// in filesCache and periodicExec unit tests.

	// work in a tmp directory
	directory, err := ioutil.TempDir("", "tmpdir")
	require.NoError(t, err)
	defer os.RemoveAll(directory)

	fileToWatch := filepath.Join(directory, "file1")

	events := make(chan struct{})
	onFilesChanged := func(files FilesModTime) (done bool, err error) {
		// just forward an event to the events channel
		events <- struct{}{}
		return false, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		done <- WatchFile(ctx, fileToWatch, onFilesChanged, 1*time.Millisecond)
	}()

	// write a file
	err = ioutil.WriteFile(filepath.Join(directory, "file1"), []byte("content"), 0644)
	require.NoError(t, err)

	// event should happen
	// since WriteFile() is not atomic, what might actually happen is:
	// 1. file gets created (no content yet)
	// 2. cache is updated with the file that has no content
	// 3. file gets content written into
	// 4. cache is updated with the file content
	// here we want to capture step 4, but need to be resilient to step 2.
	// expect 1 or 2 events in the next 500ms
	expectEvents(t, events, 1, 2, 500*time.Millisecond)

	// write another file the watcher should not care about
	err = ioutil.WriteFile(filepath.Join(directory, "file2"), []byte("content"), 0644)
	require.NoError(t, err)
	// expect 0 events in the next 200ms
	expectEvents(t, events, 0, 0, 200*time.Millisecond)

	// change first file content
	err = ioutil.WriteFile(filepath.Join(directory, "file1"), []byte("content updated"), 0644)
	require.NoError(t, err)
	// expect 1 or 2 events in the next 500ms
	expectEvents(t, events, 1, 2, 500*time.Millisecond)

	// stop watching, should return with no error
	cancel()
	require.NoError(t, <-done)
}

func Test_DirectoryWatcher(t *testing.T) {
	// Test that the directory watcher behaves as expected in the common case.
	// Mostly checking everything is correctly plugged together.
	// Specific corner cases and detailed behaviours are tested
	// in filesCache and periodicExec unit tests.

	// work in a tmp directory
	directory, err := ioutil.TempDir("", "tmpdir")
	require.NoError(t, err)
	defer os.RemoveAll(directory)

	events := make(chan struct{})
	onFilesChanged := func(files FilesModTime) (done bool, err error) {
		// just forward an event to the events channel
		events <- struct{}{}
		return false, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		done <- WatchDirectory(ctx, directory, onFilesChanged, 1*time.Millisecond)
	}()

	// write a file
	err = ioutil.WriteFile(filepath.Join(directory, "file1"), []byte("content"), 0644)
	require.NoError(t, err)

	// event should happen
	// since WriteFile() is not atomic, what might actually happen is:
	// 1. file gets created (no content yet)
	// 2. cache is updated with the file that has no content
	// 3. file gets content written into
	// 4. cache is updated with the file content
	// here we want to capture step 4, but need to be resilient to step 2.
	// expect 1 or 2 events in the next 500ms
	expectEvents(t, events, 1, 2, 500*time.Millisecond)

	// write another file
	err = ioutil.WriteFile(filepath.Join(directory, "file2"), []byte("content"), 0644)
	require.NoError(t, err)
	// expect 1 or 2 events in the next 500ms
	expectEvents(t, events, 1, 2, 500*time.Millisecond)

	// change file content
	err = ioutil.WriteFile(filepath.Join(directory, "file1"), []byte("content updated"), 0644)
	require.NoError(t, err)
	// expect 1 or 2 events in the next 500ms
	expectEvents(t, events, 1, 2, 500*time.Millisecond)

	// stop watching, should return with no error
	cancel()
	require.NoError(t, <-done)
}
