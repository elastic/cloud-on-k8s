// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package manager

import (
	"context"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/fsnotify/fsnotify"
	"os"
	"path/filepath"
	"time"
)

func readOptionalCA(path string) (*certificates.CA, error) {
	if path == "" {
		return nil, nil
	}
	return certificates.BuildCAFromFile(path)
}

func watchCADir(ctx context.Context, path string, onChange chan struct{}) error {
	if path == "" {
		return nil
	}
	cache := newCAFileModTimeCache(path)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	go func() {
		defer func() {
			onChange <- struct{}{}
			watcher.Close()
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return // channel closed
				}

				log.V(1).Info("CA watcher event", "file", event.Name, "op", event.Op)
				// Let's watch all events in the target directory and on such an event check if the two files we care
				// about have changed
				if changed := cache.Update(path); changed {
					return
				}
			case err, ok := <-watcher.Errors:
				if ok {
					log.Error(err, "CA watcher error")
				}
				return
			}
		}
	}()
	log.Info("Setting up watcher for CA path", "path", path)
	return watcher.Add(path)
}

type caFileModTimeCache map[string]time.Time

func newCAFileModTimeCache(path string) caFileModTimeCache {
	cache := caFileModTimeCache(map[string]time.Time{})
	_ = cache.Update(path)
	return cache
}

func (fmc caFileModTimeCache) Update(path string) bool {
	var updated bool
	for _, f := range []string{certificates.CertFileName, certificates.KeyFileName} {
		stat, err := os.Stat(filepath.Join(path, f))
		if err != nil {
			continue
		}
		prev, ok := fmc[f]
		if !ok {
			// initialisation does not count as update
			fmc[f] = stat.ModTime()
		}
		if prev != stat.ModTime() {
			updated = true
		}
		log.V(1).Info("file mode cache state", "file", f, "mtime", stat.ModTime(), "prev_mtime", prev)
	}
	return updated
}
