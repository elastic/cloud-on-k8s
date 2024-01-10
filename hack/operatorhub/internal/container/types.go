// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package container

// GetImagesResponse is a response to a request to return a set of images
// from the Redhat certification API.
type GetImagesResponse struct {
	Images []Image `json:"data"`
}

// gradingStatus defines the state of the image security scanning process
// within the Red Hat certification API
type gradingStatus string

const (
	gradingStatusAborted    gradingStatus = "aborted"
	gradingStatusInProgress gradingStatus = "in progress"
	gradingStatusCompleted  gradingStatus = "completed"
	gradingStatusFailed     gradingStatus = "failed"
	gradingStatusPending    gradingStatus = "pending"
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
	// ContainerGrades details the state of grading of a container image
	ContainerGrades Grade `json:"container_grades"`
	// DockerImageDigest is the SHA id of the image
	DockerImageDigest string `json:"docker_image_digest"`
}

// Grade represents the grading state of a container image.
type Grade struct {
	// Status is the grading status of the container image.
	Status gradingStatus `json:"status"`
	// StatusMessage is a message describing the grading status of the container image.
	StatusMessage string `json:"status_message"`
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
