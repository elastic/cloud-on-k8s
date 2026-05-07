// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package bucket

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/auth/credentials"
	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// deleteGCSPath deletes all objects under a GCS path using the Go SDK.
func deleteGCSPath(ctx context.Context, creds Credentials, prefix string) error {
	gcsCreds, err := credentials.NewCredentialsFromJSON(
		credentials.ServiceAccount,
		creds.GCSCredentialsJSON,
		&credentials.DetectOptions{
			Scopes: []string{"https://www.googleapis.com/auth/devstorage.read_write"},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create GCS credentials: %w", err)
	}

	client, err := storage.NewClient(ctx, option.WithAuthCredentials(gcsCreds))
	if err != nil {
		return fmt.Errorf("failed to create GCS client: %w", err)
	}
	defer client.Close()

	bucket := client.Bucket(creds.Bucket)
	it := bucket.Objects(ctx, &storage.Query{Prefix: prefix})

	var deleted int
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to list GCS objects: %w", err)
		}

		if err := bucket.Object(attrs.Name).Delete(ctx); err != nil {
			if errors.Is(err, storage.ErrObjectNotExist) {
				continue
			}
			return fmt.Errorf("failed to delete GCS object %s: %w", attrs.Name, err)
		}
		deleted++
	}

	log.Info("Deleted objects from GCS", "count", deleted, "bucket", creds.Bucket, "prefix", prefix)
	return nil
}
