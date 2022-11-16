// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package fs

import (
	"context"
	"os"
	"sync"
	"time"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("fs-watcher")

// FileWatcher watches a given set of file paths, not directories, for changes based on the file's mtime.
type FileWatcher struct {
	ctx      context.Context
	onChange func([]string)
	interval time.Duration
	cache    fileModTimeCache
	once     sync.Once
}

// NewFileWatcher creates a new file watcher, use ctx context for cancellation, paths to specify the files to watch.
// onChange is a callback to be invoked when changes are detected, a list of affected files will be passed as argument.
// interval determines how often the file watcher will try to detect changes to the files of interest.
func NewFileWatcher(ctx context.Context, paths []string, onChange func([]string), interval time.Duration) *FileWatcher {
	return &FileWatcher{
		ctx:      ctx,
		onChange: onChange,
		interval: interval,
		cache:    newFileModTimeCache(paths),
	}
}

// Run starts the file watcher. Should be typically run inside a go routine.
func (fw *FileWatcher) Run() {
	fw.once.Do(func() {
		ticker := time.NewTicker(fw.interval)
		defer ticker.Stop()
		for {
			select {
			case <-fw.ctx.Done():
				return
			case <-ticker.C:
				updated := fw.cache.Update()
				if len(updated) > 0 {
					fw.onChange(updated)
				}
			}
		}
	})
}

type fileModTimeCache map[string]time.Time

func newFileModTimeCache(paths []string) fileModTimeCache {
	cache := fileModTimeCache(map[string]time.Time{})
	for _, f := range paths {
		cache[f] = time.Time{}
	}
	_ = cache.Update()
	return cache
}

func (fmc fileModTimeCache) Update() []string {
	var updated []string
	for f, prev := range fmc {
		stat, err := os.Stat(f)
		if err != nil {
			switch {
			case os.IsNotExist(err) && !prev.IsZero():
				// file was deleted
				updated = append(updated, f)
				fmc[f] = time.Time{}
			case os.IsNotExist(err):
				// file does not exist can be ignored
			default:
				log.Error(err, "while getting file info", "file", f, "err", err.Error())
			}
			continue
		}
		if prev != stat.ModTime() {
			updated = append(updated, f)
			fmc[f] = stat.ModTime()
		}
	}
	return updated
}
