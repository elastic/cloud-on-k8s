// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package fs

import (
	"context"
	"fmt"
	"path"
	"time"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("file-watcher")
)

// Watcher reacts on changes on the filesystem
type Watcher interface {
	Run() error
}

// OnFilesChanged is a function invoked when something changed in the files.
// If it returns an error, the Watcher will stop watching and return the error.
// If it returns true, the Watcher will stop watching with no error.
type OnFilesChanged func(files FilesModTime) (done bool, err error)

// DirectoryWatcher periodically reads files in directory, and calls onFilesChanged
// on any changes in the directory's files.
// By default, it ignores hidden files and sub-directories.
func DirectoryWatcher(ctx context.Context, directory string, onFilesChanged OnFilesChanged, period time.Duration) (Watcher, error) {
	// cache all non-hidden files in the directory
	cache, err := newFilesCache(directory, true, nil)
	if err != nil {
		return nil, err
	}
	return buildWatcher(ctx, cache, onFilesChanged, period), nil
}

// FileWatcher periodically reads the given file, and calls onFileChanged
// on any changes in the file.
func FileWatcher(ctx context.Context, filepath string, onFilesChanged OnFilesChanged, period time.Duration) (Watcher, error) {
	// cache a single file
	cache, err := newFilesCache(path.Dir(filepath), false, []string{path.Base(filepath)})
	if err != nil {
		return nil, err
	}
	return buildWatcher(ctx, cache, onFilesChanged, period), nil
}

// watcher implements Watcher
type watcher struct {
	ctx    context.Context
	onExec execFunction
	period time.Duration
}

// Run reacts on changes from the filesystem
func (w *watcher) Run() error {
	return CallPeriodically(w.ctx, w.onExec, w.period)
}

// buildWatcher sets up a periodic execution to execute onFilesChanged
// when the given cache is updated.
func buildWatcher(ctx context.Context, cache *filesCache, onFilesChanged OnFilesChanged, period time.Duration) Watcher {
	// on each periodic execution, update the cache,
	// but call onFilesChanged only if the cache was updated
	var onExec = func() (done bool, err error) {
		newFiles, hasChanged, err := cache.update()
		if err != nil {
			return false, err
		}
		fmt.Println("debug", newFiles)
		if hasChanged {
			return onFilesChanged(newFiles)
		}
		// no change, continue watching
		return false, nil
	}
	return &watcher{
		ctx:    ctx,
		onExec: onExec,
		period: period,
	}
}
