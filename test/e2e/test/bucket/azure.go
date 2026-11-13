// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package bucket

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

// deleteAzurePath deletes all blobs under an Azure Blob Storage path using the Go SDK.
func deleteAzurePath(ctx context.Context, creds Credentials, prefix string) error {
	// Note: serviceURL contains the SAS token - do not log this value.
	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/?%s", creds.AzureAccount, creds.AzureSASToken)

	client, err := azblob.NewClientWithNoCredential(serviceURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create Azure client: %w", err)
	}

	var deleted int
	pager := client.NewListBlobsFlatPager(creds.Bucket, &azblob.ListBlobsFlatOptions{
		Prefix: &prefix,
	})

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list Azure blobs: %w", err)
		}

		for _, blob := range page.Segment.BlobItems {
			_, err := client.DeleteBlob(ctx, creds.Bucket, *blob.Name, nil)
			if err != nil {
				return fmt.Errorf("failed to delete Azure blob %s: %w", *blob.Name, err)
			}
			deleted++
		}
	}

	log.Info("Deleted blobs from Azure", "count", deleted, "account", creds.AzureAccount, "container", creds.Bucket, "prefix", prefix)
	return nil
}
