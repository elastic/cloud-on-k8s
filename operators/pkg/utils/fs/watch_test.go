// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package fs

import (
	"context"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// atomicFileWrite attempts to create file atomically,
// by first creating a tmp hidden file, then renaming it to the real file
// (this is atomic from the watcher point of view, not the filesystem)
func atomicFileWrite(file string, content []byte) error {
	dir := path.Dir(file)
	filename := path.Base(file)
	hiddenFilePath := filepath.Join(dir, "."+filename)
	filePath := filepath.Join(dir, filename)

	if err := ioutil.WriteFile(hiddenFilePath, content, 0644); err != nil {
		return err
	}
	return os.Rename(hiddenFilePath, filePath)
}

// expectNoEvent verifies that no event comes into the event channel for the given duration
func expectNoEvent(t *testing.T, events chan struct{}, duration time.Duration) {
	select {
	case <-events:
		t.Errorf("Got an event, but should not")
	case <-time.After(duration):
		return
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
	watcher, err := FileWatcher(ctx, fileToWatch, onFilesChanged, 1*time.Millisecond)
	require.NoError(t, err)
	done := make(chan error)
	go func() {
		done <- watcher.Run()
	}()

	// write a file
	err = atomicFileWrite(filepath.Join(directory, "file1"), []byte("content"))
	require.NoError(t, err)
	// expect an event to occur
	<-events

	// write another file the watcher should not care about
	err = atomicFileWrite(filepath.Join(directory, "file2"), []byte("content"))
	require.NoError(t, err)
	// expect no events
	expectNoEvent(t, events, 200*time.Millisecond)

	// change first file content
	err = atomicFileWrite(filepath.Join(directory, "file1"), []byte("content updated"))
	require.NoError(t, err)
	// expect an event
	<-events

	// expect no more events
	expectNoEvent(t, events, 200*time.Millisecond)

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
	watcher, err := DirectoryWatcher(ctx, directory, onFilesChanged, 1*time.Millisecond)
	require.NoError(t, err)
	done := make(chan error)
	go func() {
		done <- watcher.Run()
	}()

	// write a file
	err = atomicFileWrite(filepath.Join(directory, "file1"), []byte("content"))
	require.NoError(t, err)
	// expect an event to occur
	<-events

	// write another file
	err = atomicFileWrite(filepath.Join(directory, "file2"), []byte("content"))
	require.NoError(t, err)
	// expect an event to occur
	<-events

	// change file content
	err = atomicFileWrite(filepath.Join(directory, "file1"), []byte("content updated"))
	require.NoError(t, err)
	// expect an event to occur
	<-events

	// expect no more events
	expectNoEvent(t, events, 200*time.Millisecond)

	// stop watching, should return with no error
	cancel()
	require.NoError(t, <-done)
}
