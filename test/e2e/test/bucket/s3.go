// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package bucket

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// deleteS3Path deletes all objects under an S3 path using the Go SDK.
func deleteS3Path(ctx context.Context, creds Credentials, prefix string) error {
	client := s3.New(s3.Options{
		Region:      creds.Region,
		Credentials: credentials.NewStaticCredentialsProvider(creds.S3AccessKey, creds.S3SecretKey, ""),
	})

	var deleted int
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(creds.Bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list S3 objects: %w", err)
		}

		for _, obj := range page.Contents {
			_, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(creds.Bucket),
				Key:    obj.Key,
			})
			if err != nil {
				return fmt.Errorf("failed to delete S3 object %s: %w", *obj.Key, err)
			}
			deleted++
		}
	}

	log.Info("Deleted objects from S3", "count", deleted, "bucket", creds.Bucket, "prefix", prefix)
	return nil
}
