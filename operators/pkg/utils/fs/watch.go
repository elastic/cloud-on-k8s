// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package fs

import (
	"path"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("file-watcher")
)

// Watcher watches for changes on the filesystem
type Watcher struct {
	*periodicExec
	*filesCache
}

// OnFilesChanged is a function invoked when something changed in the FilesContent.
// If it returns an error, the Watcher will stop watching and return the error.
// If it returns true, the Watcher will stop watching with no error.
type OnFilesChanged func(files FilesContent) (done bool, err error)

// NewDirectoryWatcher periodically reads files in directory, and calls onFilesChanged
// on any changes in the directory's files.
// By default, it ignores hidden files and sub-directories.
func NewDirectoryWatcher(directory string, onFilesChanged OnFilesChanged) (*Watcher, error) {
	// cache all non-hidden files in the directory
	cache, err := newFilesCache(directory, true, nil)
	if err != nil {
		return nil, err
	}
	return buildWatcher(cache, onFilesChanged), nil
}

// NewFileWatcher periodically reads the given file, and calls onFileChanged
// on any changes in the file.
func NewFileWatcher(filepath string, onFilesChanged OnFilesChanged) (*Watcher, error) {
	// cache a single file
	cache, err := newFilesCache(path.Dir(filepath), false, []string{path.Base(filepath)})
	if err != nil {
		return nil, err
	}
	return buildWatcher(cache, onFilesChanged), nil
}

// buildWatcher sets up a periodicExec to execute onFilesChange
// when the given cache is updated, and returns it as a Watcher.
func buildWatcher(cache *filesCache, onFilesChanged OnFilesChanged) *Watcher {
	// on each periodic execution, update the cache,
	// but call onFilesChanged only if the cache was updated
	var onExec = func() (done bool, err error) {
		newFiles, hasChanged, err := cache.update()
		if err != nil {
			return false, err
		}
		if hasChanged {
			return onFilesChanged(newFiles)
		}
		// no change, continue watching
		return false, nil
	}
	return &Watcher{
		periodicExec: newPeriodicExec(onExec),
		filesCache:   cache,
	}
}
