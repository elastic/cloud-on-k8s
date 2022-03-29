// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package manager

import (
	"context"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/fsnotify/fsnotify"
	"path/filepath"
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
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	go func() {
		defer watcher.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return // channel closed
				}
				// TODO verify rename is relevant
				const relevantOps = fsnotify.Create | fsnotify.Write | fsnotify.Remove | fsnotify.Rename
				// TODO verify we need to handle symlinks b/c k8s secret mounts etc
				affectedFilePath := filepath.Clean(event.Name)
				affectedFileName := filepath.Base(affectedFilePath)
				if (affectedFileName == certificates.KeyFileName || affectedFileName == certificates.CertFileName) &&
					event.Op&relevantOps != 0 {
					log.Info("CA file changed", "file", affectedFilePath, "op", event.Op)
					onChange <- struct{}{}
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
