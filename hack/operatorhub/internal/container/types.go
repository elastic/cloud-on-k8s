// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package container

// GetImagesResponse is a response to a request to return a set of images
// from the Redhat certification API.
type GetImagesResponse struct {
	Images []Image `json:"data"`
}

// scanStatus defines the state of the image scanning process
// within the Red Hat certification API
type scanStatus string

const (
	scanStatusInProgress scanStatus = "in progress"
	scanStatusPassed     scanStatus = "passed"
	scanStatusFailed     scanStatus = "failed"
)

// Image represents a Redhat certification API response
// representing a container image.
type Image struct {
	// ID is the id of the image.
	ID string `json:"_id"`
	// Architecture is the architecture (amd64, arm64, etc).
	Architecture *string `json:"architecture"`
	// Repositories is a slice of Repository structs
	Repositories []Repository `json:"repositories"`
	// ScanStatus is the status indicating whether the image has been scanned.
	ScanStatus scanStatus `json:"scan_status"`
	// DockerImageDigest is the SHA id of the image
	DockerImageDigest string `json:"docker_image_digest"`
}

// Repository represents an image repository, and any tags applied to a container image.
type Repository struct {
	// Repository is the repository name.
	Repository string `json:"repository"`
	// Tags are any tags applied to this image/repository combination.
	Tags Tags `json:"tags"`
}

type Tags []Tag

// Tag represents a tag of a container image.
type Tag struct {
	Name string `json:"name"`
}

func (ts Tags) contains(tag Tag) bool {
	for _, t := range ts {
		if tag.Name == t.Name {
			return true
		}
	}
	return false
}
