// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
)

var imagesDigestsSingleton *imagesDigests

type imagesDigests struct {
	images map[string]string

	// lock is used to protect access to the map. It's less efficient than sync.RWMutex, however we can assume a low
	// performance impact given that only a few digests will be actually retrieved during an E2E test session.
	lock sync.Locker
}

func init() {
	imagesDigestsSingleton = &imagesDigests{
		images: make(map[string]string),
		lock:   &sync.Mutex{},
	}
}

// backoff is a backoff policy used to retry access to the image registry.
var backoff = wait.Backoff{
	Duration: 1 * time.Second,
	Factor:   2,
	Steps:    10,
}

// retriable attempts to detect if, given an error, we should try again to get the image digest.
var retriable = func(err error) bool {
	// Do not retry on permission errors or if the resource is not found.
	return !(client.IsForbidden(err) || client.IsUnauthorized(err) || client.IsNotFound(err))
}

// WithDigestOrDie returns a fully qualified image name with the version (tag) replaced by the image digest.
// Retries are attempted on a best effort basis. This function panics once the maximum retries is reached.
func WithDigestOrDie(image container.Image, tag string) (withDigest string) {
	err := retry.OnError(
		backoff,
		retriable,
		func() error {
			digest, err := getDigest(image, tag)
			if err != nil {
				logf.Log.Error(
					err,
					"Error while fetching image digest",
					"image", image,
					"tag", tag,
					"retriable", retriable(err),
				)
				return err
			}
			withDigest = fmt.Sprintf("%s/%s@%s", container.DefaultContainerRegistry, image, digest)
			return nil
		},
	)
	if err != nil {
		// We eventually panic as it is likely to prevent any subsequent test to run.
		panic(err)
	}
	return
}

func getDigest(image container.Image, tag string) (string, error) {
	imagesDigestsSingleton.lock.Lock()
	defer imagesDigestsSingleton.lock.Unlock()

	taggedImage := fmt.Sprintf("%s:%s", image, tag)
	imageDigest := imagesDigestsSingleton.images[taggedImage]
	if imageDigest != "" {
		logf.Log.Info("Reusing image digest from cache", "image", image, "tag", tag, "digest", imageDigest)
		return imageDigest, nil
	}

	withRegistry := fmt.Sprintf("%s/%s", container.DefaultContainerRegistry, taggedImage)
	digest, err := crane.Digest(withRegistry)
	if err != nil {
		return "", err
	}

	// Keep the digest in cache.
	imagesDigestsSingleton.images[taggedImage] = digest
	logf.Log.Info("Fetched image digest from registry", "image", image, "tag", tag, "digest", digest)
	return digest, nil
}
