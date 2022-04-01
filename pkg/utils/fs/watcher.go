// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package fs

import (
	"context"
	"os"
	"time"
)

type fileWatcher struct {
	ctx      context.Context
	onChange func([]string)
	period   time.Duration
	cache    fileModTimeCache
}

func NewFileWatcher(ctx context.Context, paths []string, onChange func([]string), period time.Duration) *fileWatcher {
	return &fileWatcher{
		ctx:      ctx,
		onChange: onChange,
		period:   period,
		cache:    newFileModTimeCache(paths),
	}
}

func (fw *fileWatcher) Run() {
	ticker := time.NewTicker(fw.period)
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
			if os.IsNotExist(err) && !prev.IsZero() {
				// file was deleted
				updated = append(updated, f)
				fmc[f] = time.Time{}
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
