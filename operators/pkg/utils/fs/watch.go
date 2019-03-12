// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package fs

import (
	"context"
	"path"
	"time"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("file-watcher")
)

// OnFilesChanged is a function invoked when something changed in the F.
// If it returns an error, the Watcher will stop watching and return the error.
// If it returns true, the Watcher will stop watching with no error.
type OnFilesChanged func(files FilesModTime) (done bool, err error)

// WatchDirectory periodically reads files in directory, and calls onFilesChanged
// on any changes in the directory's files.
// By default, it ignores hidden files and sub-directories.
func WatchDirectory(ctx context.Context, directory string, onFilesChanged OnFilesChanged, period time.Duration) error {
	// cache all non-hidden files in the directory
	cache, err := newFilesCache(directory, true, nil)
	if err != nil {
		return err
	}
	return periodicWatch(ctx, cache, onFilesChanged, period)
}

// WatchFile periodically reads the given file, and calls onFileChanged
// on any changes in the file.
func WatchFile(ctx context.Context, filepath string, onFilesChanged OnFilesChanged, period time.Duration) error {
	// cache a single file
	cache, err := newFilesCache(path.Dir(filepath), false, []string{path.Base(filepath)})
	if err != nil {
		return err
	}
	return periodicWatch(ctx, cache, onFilesChanged, period)
}

// periodicWatch sets up a periodic execution to execute onFilesChange
// when the given cache is updated.
func periodicWatch(ctx context.Context, cache *filesCache, onFilesChanged OnFilesChanged, period time.Duration) error {
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
	return CallPeriodically(ctx, onExec, period)
}
