// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package fs

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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

	events := make(chan FilesContent)

	onFilesChanged := func(files FilesContent) (done bool, err error) {
		// just forward files we got to the events channel
		events <- files
		return false, nil
	}

	watcher, err := NewFileWatcher(fileToWatch, onFilesChanged, 1*time.Millisecond)
	require.NoError(t, err)

	done := make(chan error)
	go func() {
		done <- watcher.Run()
	}()

	// write a file
	with1File := FilesContent{
		"file1": []byte("content1"),
	}
	err = ioutil.WriteFile(filepath.Join(directory, "file1"), with1File["file1"], 0644)
	require.NoError(t, err)

	// event should happen
	// since WriteFile() is not atomic, what might actually happen is:
	// 1. file gets created (no content yet)
	// 2. cache is updated with the file that has no content
	// 3. file gets content written into
	// 4. cache is updated with the file content
	// here we want to capture step 4, but need to be resilient to step 2.
	for {
		evt := <-events
		if evt.Equals(FilesContent{"file1": []byte{}}) {
			continue // step 2
		} else {
			require.True(t, evt.Equals(with1File)) // step 4
			break
		}
	}

	// write another file the watcher should not care about
	err = ioutil.WriteFile(filepath.Join(directory, "file2"), []byte("content"), 0644)
	require.NoError(t, err)

	// change first file content
	updated := FilesContent{
		"file1": []byte("content1updated"),
	}
	err = ioutil.WriteFile(filepath.Join(directory, "file1"), updated["file1"], 0644)
	require.NoError(t, err)

	// event should happen for file1
	// since WriteFile() is not atomic, what might actually happen is:
	// 1. file gets truncated
	// 2. cache is updated with the file that has no content
	// 3. file gets content written into
	// 4. cache is updated with the file content
	// here we want to capture step 4, but need to be resilient to step 2.
	for {
		evt := <-events
		if evt.Equals(FilesContent{"file1": []byte{}}) {
			continue // step 2
		} else {
			require.True(t, evt.Equals(updated)) // step 4
			break
		}
	}
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

	events := make(chan FilesContent)

	onFilesChanged := func(files FilesContent) (done bool, err error) {
		// just forward files we got to the events channel
		events <- files
		return false, nil
	}

	watcher, err := NewDirectoryWatcher(directory, onFilesChanged, 1*time.Millisecond)
	require.NoError(t, err)

	done := make(chan error)
	go func() {
		done <- watcher.Run()
	}()

	// write a file
	with1File := FilesContent{
		"file1": []byte("content1"),
	}
	err = ioutil.WriteFile(filepath.Join(directory, "file1"), with1File["file1"], 0644)
	require.NoError(t, err)

	// event should happen
	// since WriteFile() is not atomic, what might actually happen is:
	// 1. file gets created (no content yet)
	// 2. cache is updated with the file that has no content
	// 3. file gets content written into
	// 4. cache is updated with the file content
	// here we want to capture step 4, but need to be resilient to step 2.
	for {
		evt := <-events
		if evt.Equals(FilesContent{"file1": []byte{}}) {
			continue // step 2
		} else {
			fmt.Println(evt)
			require.True(t, evt.Equals(with1File)) // step 4
			break
		}
	}

	// write another file
	with2Files := FilesContent{
		"file1": []byte("content1"),
		"file2": []byte("content2"),
	}
	err = ioutil.WriteFile(filepath.Join(directory, "file2"), with2Files["file2"], 0644)
	require.NoError(t, err)

	// event should happen
	// since WriteFile() is not atomic, what might actually happen is:
	// 1. file gets created (no content yet)
	// 2. cache is updated with the file that has no content
	// 3. file gets content written into
	// 4. cache is updated with the file content
	// here we want to capture step 4, but need to be resilient to step 2.
	for {
		evt := <-events
		if evt.Equals(FilesContent{"file1": []byte("content1"), "file2": []byte("")}) {
			continue // step 2
		} else {
			require.True(t, evt.Equals(with2Files)) // step 4
			break
		}
	}

	// change file content
	updated := FilesContent{
		"file1": []byte("content1updated"),
		"file2": []byte("content2"),
	}
	err = ioutil.WriteFile(filepath.Join(directory, "file1"), updated["file1"], 0644)
	require.NoError(t, err)

	// event should happen
	// since WriteFile() is not atomic, what might actually happen is:
	// 1. file gets truncated
	// 2. cache is updated with the file that has no content
	// 3. file gets content written into
	// 4. cache is updated with the file content
	// here we want to capture step 4, but need to be resilient to step 2.
	for {
		evt := <-events
		if evt.Equals(FilesContent{"file1": []byte(""), "file2": []byte("content2")}) {
			continue // step 2
		} else {
			require.True(t, evt.Equals(updated)) // step 4
			break
		}
	}

	// stop watcher, should return with no error
	watcher.Stop()
	require.NoError(t, <-done)
}
