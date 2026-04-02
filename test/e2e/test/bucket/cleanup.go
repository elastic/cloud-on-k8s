// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package bucket

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log logr.Logger

func init() {
	log = logf.Log.WithName("bucket-cleanup")
}

// ensureTrailingSlash ensures the path ends with a slash.
// This is important for cloud storage prefix matching to only match
// objects within a specific "directory" and not objects that happen
// to start with the same characters.
func ensureTrailingSlash(path string) string {
	if !strings.HasSuffix(path, "/") {
		return path + "/"
	}
	return path
}

// DeletePath deletes all objects under the given path prefix in a cloud storage bucket.
// The path should be relative to the bucket root (no bucket name, no leading slash).
// Returns nil if deletion succeeds or path doesn't exist (idempotent).
// A 5-minute timeout is applied to the operation.
func DeletePath(ctx context.Context, creds Credentials, path string) error {
	if path == "" {
		return nil
	}

	// Normalize path: remove leading slash, add trailing slash.
	// The trailing slash ensures prefix matching only finds objects within this
	// "directory", not objects that happen to share the same prefix
	// (e.g., "foo/bar/" matches "foo/bar/baz" but not "foo/bar-other/baz").
	path = strings.TrimPrefix(path, "/")
	path = ensureTrailingSlash(path)

	log.Info("Deleting bucket path", "bucket", creds.Bucket, "path", path, "provider", creds.Provider)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	switch creds.Provider {
	case "gcs":
		return deleteGCSPath(ctx, creds, path)
	case "s3":
		return deleteS3Path(ctx, creds, path)
	case "azure":
		return deleteAzurePath(ctx, creds, path)
	default:
		return fmt.Errorf("unsupported provider: %s", creds.Provider)
	}
}
