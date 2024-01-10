// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package container

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

const (
	eckOperatorFormat              = "docker.elastic.co/eck/eck-operator-ubi:%s"
	registryURL                    = "quay.io"
	httpAcceptHeader               = "Accept"
	httpContentTypeHeader          = "Content-Type"
	httpXAPIKeyHeader              = "X-API-KEY"
	httpApplicationJSONHeaderValue = "application/json"
	getImagesFilter                = "repositories.tags.name==%s"
	publishOperation               = "publish"
	syncTagsOperation              = "sync-tags"
)

var (
	errImageNotFound             = fmt.Errorf("image not found")
	catalogAPIURL                = "https://catalog.redhat.com/api/containers/v1"
	eckOperatorRegistryReference = "%s/redhat-isv-containers/%s:%s"
	getCatalogImagesURL          = "%s/projects/certification/id/%s/images"
	registryUserFormat           = "redhat-isv-containers+%s-robot"
	imagePublishURL              = "%s/projects/certification/id/%s/requests/images"
	latestTag                    = Tag{Name: "latest"}
)

// CommonConfig are common configuration options between
// the push and publish commands.
type CommonConfig struct {
	DryRun              bool
	ProjectID           string
	RedhatCatalogAPIKey string
	RegistryPassword    string
}

// PushImage will push an image to the Quay.io registry if it is determined
// that the image doesn't already exist.  If 'Force' option is used, the
// image will be pushed regardless if it already exists.
func PushImage(c CommonConfig, newTag Tag, force bool) error {
	log.Printf("Determining if image already exists within project with tag: %s", newTag.Name)
	exists, err := imageExistsInProject(c, newTag)
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("failed to determine if image exists: %w", err)
	}
	if exists && !force {
		log.Println("✓")
		log.Println("not continuing as image was already found within redhat project")
		return nil
	}
	if force {
		log.Println("pushing image as force was set")
	}

	if err = pushImageToRegistry(c, newTag.Name); err != nil {
		log.Println("x")
		return fmt.Errorf("while pushing image: %w", err)
	}
	log.Println("✓")
	if err = syncImagesTaggedAsLatest(c, newTag); err != nil {
		log.Println("x")
		return fmt.Errorf("while syncing images tagged as latest: %w", err)
	}
	log.Println("✓")
	return nil
}

// PublishImage will publish an existing image in the redhat catalog,
// if the image has completed the scan process.  It will wait up to
// 'ImageScanTimeout' for image to have completed the scan before failing.
func PublishImage(c CommonConfig, newTag Tag, imageScanTimeout time.Duration) error {
	return publishImageInProject(c, newTag, imageScanTimeout)
}

// imageExistsInProject will determine whether an image with the given tag exists in the certification api.
func imageExistsInProject(c CommonConfig, tag Tag) (bool, error) {
	images, err := getImagesByTag(c, tag.Name)
	if err != nil && errors.Is(err, errImageNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err == nil && len(images) == 0 {
		return false, nil
	}
	if getFirstUndeletedImage(images) == nil {
		log.Println("ignoring existing potentially deleted image")
		return false, nil
	}
	return true, nil
}

// GetImageSHA will query the Red Hat certification API, returning a list of images for a given
// tag, and return the SHA of the image to be used in the manifests.
func GetImageSHA(c CommonConfig, tag string) (string, error) {
	var imageSHA string
	images, err := getImagesByTag(c, tag)
	if err != nil && errors.Is(err, errImageNotFound) {
		return imageSHA, nil
	}
	if err != nil {
		return imageSHA, err
	}
	if err == nil && len(images) == 0 {
		return imageSHA, nil
	}
	image := getFirstUndeletedImage(images)
	if image == nil {
		return imageSHA, fmt.Errorf("couldn't find image with tag: %s", tag)
	}
	return image.DockerImageDigest, nil
}

// defaultHTTPClient will return an http client with a 60 second timeout
// if the calling package does not provide an http client to this package.
func defaultHTTPClient() *http.Client {
	return &http.Client{
		// This timeout is set high, as the redhat certification api
		// sometimes is very slow to respond.
		Timeout: 60 * time.Second,
	}
}

// getImages will return a slice of Images from the Red Hat Certification API
// while filtering using the given tag, returning any errors it encounters.
func getImagesByTag(c CommonConfig, tag string) ([]Image, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	url := fmt.Sprintf(getCatalogImagesURL, catalogAPIURL, c.ProjectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for url (%s): %w", url, err)
	}

	q := req.URL.Query()
	q.Add("filter", fmt.Sprintf(getImagesFilter, tag))
	req.URL.RawQuery = q.Encode()

	addHeaders(req, c.RedhatCatalogAPIKey)

	var res *http.Response
	if res, err = defaultHTTPClient().Do(req); err != nil {
		return nil, fmt.Errorf("failed to request whether image exists: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode > 299 {
		if bodyBytes, err := io.ReadAll(res.Body); err != nil {
			return nil, fmt.Errorf("failed to check whether image exists, body: %s, code: %d", string(bodyBytes), res.StatusCode)
		}
		return nil, fmt.Errorf("failed to check whether image exists, code: %d", res.StatusCode)
	}
	var bodyBytes []byte
	if bodyBytes, err = io.ReadAll(res.Body); err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}
	response := GetImagesResponse{}
	if err = json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal body into valid response: %w", err)
	}
	return response.Images, nil
}

// addHeaders will add the required headers to communicate with
// the Red Hat certification API.
func addHeaders(req *http.Request, apiKey string) {
	req.Header.Add(httpAcceptHeader, httpApplicationJSONHeaderValue)
	req.Header.Add(httpContentTypeHeader, httpApplicationJSONHeaderValue)
	req.Header.Add(httpXAPIKeyHeader, apiKey)
}

// getFirstUndeletedImage return the first undeleted image returned from the redhat certification api.
// Images that are deleted from the Red Hat certification API are still returned, but have no architecture
// defined in the output, so we use that to determine if an image has been deleted.
func getFirstUndeletedImage(images []Image) *Image {
	for _, image := range images {
		if image.Architecture != nil {
			return &image
		}
	}
	return nil
}

// pushImageToRegistry will use the crane tool to pull the ECK operator
// container image locally, and push it to Quay.io registry using the
// provided credentials.
func pushImageToRegistry(c CommonConfig, tag string) error {
	if c.DryRun {
		log.Printf("not pushing image as dry-run is set.")
		return nil
	}

	username := fmt.Sprintf(registryUserFormat, c.ProjectID)
	formattedEckOperatorRedhatReference := fmt.Sprintf(eckOperatorRegistryReference, registryURL, c.ProjectID, tag)
	imageToPull := fmt.Sprintf(eckOperatorFormat, tag)

	log.Printf("pulling image (%s) in preparation to push (%s) to quay.io registry: ", imageToPull, formattedEckOperatorRedhatReference)
	// default credentials are used here, as the operator image which we use as a source is public.
	image, err := crane.Pull(imageToPull, crane.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("while pulling (%s): %w", imageToPull, err)
	}
	log.Println("✓")
	log.Printf("pushing image (%s) to quay.io registry\n", formattedEckOperatorRedhatReference)
	err = crane.Push(
		image,
		formattedEckOperatorRedhatReference,
		crane.WithAuth(&authn.Basic{
			Username: username,
			Password: c.RegistryPassword}),
		crane.WithPlatform(&v1.Platform{
			OS:           "linux",
			Architecture: "amd64"}),
	)
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("while pushing (%s): %w", formattedEckOperatorRedhatReference, err)
	}
	log.Println("✓")
	// Since we only push when dry-run isn't set, go ahead and
	// tag this image as 'latest' such that it shows up at the
	// top of the RedHat Catalog.
	log.Printf("tagging (%s) as 'latest' in quay.io registry\n", formattedEckOperatorRedhatReference)
	err = crane.Tag(
		formattedEckOperatorRedhatReference, latestTag.Name,
		crane.WithAuth(&authn.Basic{
			Username: username,
			Password: c.RegistryPassword}),
	)
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("while tagging (%s) as 'latest': %w", formattedEckOperatorRedhatReference, err)
	}
	log.Println("✓")
	return nil
}

// syncImagesTaggedAsLatest will get all images tagged as "latest",
// and for any that are not the current tag, we run a "sync-tags"
// operation to remove the "latest" tag from the api cache.
func syncImagesTaggedAsLatest(c CommonConfig, newTag Tag) error {
	if c.DryRun {
		return nil
	}

	images, err := getImagesByTag(c, newTag.Name)
	if err != nil {
		return fmt.Errorf("while syncing tags for images marked as 'latest': %w", err)
	}
	for _, image := range images {
		for _, repo := range image.Repositories {
			if repo.Tags.contains(latestTag) && !repo.Tags.contains(newTag) {
				img := image
				if err := doOperationForImage(&img, c, newTag.Name, syncTagsOperation); err != nil {
					return fmt.Errorf("while syncing tags for image %s: %w", image.ID, err)
				}
				continue
			}
		}
	}
	return nil
}

// publishImageInProject will wait until the image with the given tag has been graded, and then attempt to publish
// the image within the Red Hat certification API. If imageScanTimeout is reached waiting for the image to
// be set as graded within the API an error will be returned.
func publishImageInProject(c CommonConfig, newTag Tag, imageScanTimeout time.Duration) error {
	ticker := time.NewTicker(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), imageScanTimeout)
	defer cancel()

	log.Printf("waiting for image to complete grading process... ")
	image, done, err := hasBeenGraded(c, newTag.Name)
	if err != nil {
		return err
	}
	if done {
		if c.DryRun {
			log.Printf("not publishing image as dry-run is set")
			return nil
		}
		return doOperationForImage(image, c, newTag.Name, publishOperation)
	}

	for {
		select {
		case <-ticker.C:
			image, done, err := hasBeenGraded(c, newTag.Name)
			if err != nil {
				return err
			}
			if !done {
				continue
			}
			if c.DryRun {
				log.Printf("not publishing image as dry-run is set")
				return nil
			}
			return doOperationForImage(image, c, newTag.Name, publishOperation)

		case <-ctx.Done():
			return fmt.Errorf("image scan not completed within timeout of %s", imageScanTimeout)
		}
	}
}

// hasBeenGraded will get the first valid image tag within the Red Hat certification API
// and ensure that the grades status is set to "completed", returning the image.
func hasBeenGraded(c CommonConfig, tag string) (*Image, bool, error) {
	images, err := getImagesByTag(c, tag)
	if err != nil {
		log.Printf("failed to find image in redhat catalog api, retrying: %s", err)
		return nil, false, nil
	}
	if len(images) == 0 {
		return nil, false, nil
	}
	image := getFirstUndeletedImage(images)
	if image == nil {
		return nil, false, nil
	}
	switch image.ContainerGrades.Status {
	case gradingStatusCompleted:
		log.Println("✓")
		return image, true, nil
	case gradingStatusFailed:
		return nil, true, fmt.Errorf("image grading failed: message: %s", image.ContainerGrades.StatusMessage)
	case gradingStatusInProgress:
		log.Println("grading still in progress")
		return nil, false, nil
	case gradingStatusAborted:
		return nil, true, fmt.Errorf("image grading aborted: message: %s", image.ContainerGrades.StatusMessage)
	case gradingStatusPending:
		log.Println("grading pending")
		return nil, false, nil
	}
	return nil, false, nil
}

// doOperationForImage will perform a given operation on an image with the given tag in the Red Hat certification API.
//
// Utilized operations:
// 1) publish
// 2) sync-tags
func doOperationForImage(image *Image, c CommonConfig, newTag string, operation string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	log.Printf("operation %s for image (%s), tag %s: ", operation, image.ID, newTag)
	url := fmt.Sprintf(imagePublishURL, catalogAPIURL, c.ProjectID)
	var body = []byte(fmt.Sprintf(`{"image_id": "%s", "operation": "%s"}`, image.ID, operation))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("failed to create request to publish image: %w", err)
	}

	addHeaders(req, c.RedhatCatalogAPIKey)

	res, err := defaultHTTPClient().Do(req)
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("failed request to publish image: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode > 299 {
		log.Println("ⅹ")
		if bodyBytes, err := io.ReadAll(res.Body); err != nil {
			return fmt.Errorf("failed request to publish image, body: %s, code: %d", string(bodyBytes), res.StatusCode)
		}
		return fmt.Errorf("failed request to publish image, code: %d", res.StatusCode)
	}
	log.Println("✓")
	return nil
}
