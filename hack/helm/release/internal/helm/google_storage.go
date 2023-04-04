// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package helm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	storage "cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
)

const (
	indexFileName    = "index.yaml"
	oldIndexFileName = "index.yaml.old"
)

type readIndexConfig struct {
	client            *storage.Client
	indexFile, bucket string
}

type writeIndexConfig struct {
	client                                *storage.Client
	bucketFileName, bucket, chartsRepoURL string
	existingIndex                         *index
	indexFileHandle                       *os.File
}

// readIndexFromBucket will read `index.yaml` from given bucket
// into a named `indexFile` and return the index object with the
// full file path and the generation of the index.yaml file returned
// by the google storage client.
func readIndexFromBucket(ctx context.Context, config readIndexConfig) (*index, error) {
	f, err := os.Create(config.indexFile)
	if err != nil {
		return nil, fmt.Errorf("while creating empty index.yaml: %w", err)
	}

	log.Printf("Attempting to read %s from bucket: %s", indexFileName, config.bucket)
	o := config.client.Bucket(config.bucket).Object(indexFileName)
	attrs, err := o.Attrs(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading attributes of %s: %w", indexFileName, err)
	}
	reader, err := o.NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("while creating new reader for %s: %w", indexFileName, err)
	}
	defer reader.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return nil, fmt.Errorf("while writing %s: %w", oldIndexFileName, err)
	}

	if err = f.Close(); err != nil {
		return nil, fmt.Errorf("while closing %s file: %w", oldIndexFileName, err)
	}
	// create a new index object from the index just read from bucket
	// to ensure that the file is the same generation when the write
	// of the new index occurs after update.
	return &index{path: config.indexFile, generation: attrs.Generation}, nil
}

// writeIndexToBucket will write a given `bucketFileName` to given bucket
// into a named `indexFile` with the contents of `config.indexFileHandle`
// according to the following rules:
//
//  1. if `config.existingIndex` is given, it will ensure that the generation
//     of the file to be written to the bucket is the same generation
//     as the given `existingIndex.generation`, and fail it they do not match.
//  2. if `config.existingIndex` is not given, the file will be written to the
//     bucket without checking the existing generation of the file.
//
// See https://cloud.google.com/storage/docs/request-preconditions#json-api for details
// of how google storage handles request preconditions.
func writeIndexToBucket(ctx context.Context, config writeIndexConfig) (*index, error) {
	bkt := config.client.Bucket(config.bucket)
	o := bkt.Object(config.bucketFileName)

	if config.existingIndex != nil {
		log.Printf("Ensuring that current %s in bucket is at generation (%d)", indexFileName, config.existingIndex.generation)
		o = o.If(storage.Conditions{GenerationMatch: config.existingIndex.generation})
	}

	log.Printf("Writing new remote helm index file to bucket for %s", config.chartsRepoURL)
	writer := o.NewWriter(ctx)

	if _, err := io.Copy(writer, config.indexFileHandle); err != nil {
		return nil, fmt.Errorf("while copying new %s to bucket: %w", indexFileName, err)
	}

	if err := writer.Close(); err != nil {
		var e *googleapi.Error
		if errors.As(err, &e) {
			if e.Code == http.StatusPreconditionFailed {
				return nil, fmt.Errorf("while writing new %s to bucket: %w", indexFileName, err)
			}
		}
		return nil, fmt.Errorf("while writing new %s to bucket: %w", indexFileName, err)
	}

	attrs := writer.Attrs()
	log.Printf("Wrote new index to bucket at generation: %d", attrs.Generation)
	return &index{path: config.indexFileHandle.Name(), generation: attrs.Generation}, nil
}
