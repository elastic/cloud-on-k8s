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
	eckOperatorFormat              = "docker.elastic.co/eck/eck-operator-ubi8:%s"
	registryURL                    = "quay.io"
	httpAcceptHeader               = "Accept"
	httpContentTypeHeader          = "Content-Type"
	httpXAPIKeyHeader              = "X-API-KEY"
	httpApplicationJSONHeaderValue = "application/json"
	getImagesFilter                = "repositories.tags.name==%s"
)

var (
	errImageNotFound             = fmt.Errorf("image not found")
	catalogAPIURL                = "https://catalog.redhat.com/api/containers/v1"
	eckOperatorRegistryReference = "%s/redhat-isv-containers/%s:%s"
	getCatalogImagesURL          = "%s/projects/certification/id/%s/images"
	registryUserFormat           = "redhat-isv-containers+%s-robot"
	imagePublishURL              = "%s/projects/certification/id/%s/requests/images"
)

// CommonConfig are common configuration options between
// the push and publish commands.
type CommonConfig struct {
	DryRun              bool
	HTTPClient          *http.Client
	ProjectID           string
	RedhatCatalogAPIKey string
	RegistryPassword    string
	Tag                 string
}

// PushConfig is the configuration required for the push image command.
type PushConfig struct {
	CommonConfig
	Force bool
}

// PublishConfig is the configuration required for the publish command.
type PublishConfig struct {
	CommonConfig
	ImageScanTimeout time.Duration
}

// PushImage will push an image to the Quay.io registry if it is determined
// that the image doesn't already exist.  If 'Force' option is used, the
// image will be pushed regardless if it already exists.
func PushImage(c PushConfig) error {
	if c.HTTPClient == nil {
		c.HTTPClient = defaultHTTPClient()
	}

	log.Printf("Determining if image already exists within project with tag: %s", c.Tag)
	exists, err := ImageExistsInProject(c.HTTPClient, c.RedhatCatalogAPIKey, c.ProjectID, c.Tag)
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("failed to determine if image exists: %w", err)
	}
	if exists && !c.Force {
		log.Println("✓")
		log.Println("not continuing as image was already found within redhat project")
		return nil
	}
	if c.Force {
		log.Println("pushing image as force was set")
	}

	if err = pushImageToRegistry(c); err != nil {
		log.Println("x")
		return fmt.Errorf("failed to push image: %w", err)
	}
	log.Println("✓")
	return nil
}

// PublishImage will publish an existing image in the redhat catalog,
// if the image has completed the scan process.  It will wait up to
// 'ImageScanTimeout' for image to have completed the scan before failing.
func PublishImage(c PublishConfig) error {
	if c.HTTPClient == nil {
		c.HTTPClient = defaultHTTPClient()
	}

	return publishImageInProject(c.HTTPClient, c.ImageScanTimeout, c.RedhatCatalogAPIKey, c.ProjectID, c.Tag, c.DryRun)
}

// ImageExistsInProject will determine whether an image with the given tag exists in the certification api.
func ImageExistsInProject(httpClient *http.Client, apiKey, projectID, tag string) (bool, error) {
	images, err := getImages(httpClient, apiKey, projectID, tag)
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
func GetImageSHA(httpClient *http.Client, apiKey, projectID, tag string) (string, error) {
	if httpClient == nil {
		httpClient = defaultHTTPClient()
	}
	var imageSHA string
	images, err := getImages(httpClient, apiKey, projectID, tag)
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
func getImages(httpClient *http.Client, apiKey, projectID, tag string) ([]Image, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	url := fmt.Sprintf(getCatalogImagesURL, catalogAPIURL, projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for url (%s): %w", url, err)
	}

	q := req.URL.Query()
	q.Add("filter", fmt.Sprintf(getImagesFilter, tag))
	req.URL.RawQuery = q.Encode()

	addHeaders(req, apiKey)

	var res *http.Response
	if res, err = httpClient.Do(req); err != nil {
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
func pushImageToRegistry(c PushConfig) error {
	if c.DryRun {
		log.Printf("not pushing image as dry-run is set.")
		return nil
	}

	username := fmt.Sprintf(registryUserFormat, c.ProjectID)
	formattedEckOperatorRedhatReference := fmt.Sprintf(eckOperatorRegistryReference, registryURL, c.ProjectID, c.Tag)
	imageToPull := fmt.Sprintf(eckOperatorFormat, c.Tag)

	log.Printf("pulling image (%s) in preparation to push (%s) to quay.io registry: ", imageToPull, formattedEckOperatorRedhatReference)
	// default credentials are used here, as the operator image which we use as a source is public.
	image, err := crane.Pull(imageToPull, crane.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("while pulling (%s): %w", imageToPull, err)
	}
	log.Printf("pushing image (%s) to quay.io registry: ", formattedEckOperatorRedhatReference)
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
	return nil
}

// publishImageInProject will wait until the image with the given tag is scanned, and then attempt to publish
// the image within the Red Hat certification API.  If imageScanTimeout is reached waiting for the image to
// be set as scanned within the API and error will be returned.
func publishImageInProject(httpClient *http.Client, imageScanTimeout time.Duration, apiKey, projectID, tag string, dryRun bool) error {
	ticker := time.NewTicker(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), imageScanTimeout)
	defer cancel()

	log.Printf("waiting for image to complete scan process... ")
	image, done, err := isImageScanned(httpClient, apiKey, projectID, tag)
	if err != nil {
		return err
	}
	if done {
		if dryRun {
			log.Printf("not publishing image as dry-run is set")
			return nil
		}
		return doPublish(image, httpClient, apiKey, projectID, tag)
	}

	for {
		select {
		case <-ticker.C:
			image, done, err := isImageScanned(httpClient, apiKey, projectID, tag)
			if err != nil {
				return err
			}
			if !done {
				continue
			}
			if dryRun {
				log.Printf("not publishing image as dry-run is set")
				return nil
			}
			return doPublish(image, httpClient, apiKey, projectID, tag)

		case <-ctx.Done():
			return fmt.Errorf("image scan not completed within timeout of %s", imageScanTimeout)
		}
	}
}

// isImageScanned will get the first valid image tag within the Red Hat certification API
// and ensure that the scan status is set to "passed", returning the image.
func isImageScanned(httpClient *http.Client, apiKey, projectID, tag string) (image *Image, done bool, err error) {
	images, err := getImages(httpClient, apiKey, projectID, tag)
	if err != nil {
		log.Printf("failed to find image in redhat catlog api, retrying: %s", err)
		return nil, false, nil
	}
	if len(images) == 0 {
		return nil, false, nil
	}
	image = getFirstUndeletedImage(images)
	if image == nil {
		return nil, false, nil
	}
	switch image.ScanStatus {
	case scanStatusPassed:
		log.Println("✓")
		return image, true, nil
	case scanStatusFailed:
		return nil, true, fmt.Errorf("image scan failed")
	case scanStatusInProgress:
		log.Println("scan still in progress")
		return nil, false, nil
	}
	return nil, false, nil
}

// doPublish will publish an image with the given tag in the Red Hat certification API.
func doPublish(image *Image, httpClient *http.Client, apiKey, projectID, tag string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	log.Printf("publishing image (%s), tag %s: ", image.ID, tag)
	url := fmt.Sprintf(imagePublishURL, catalogAPIURL, projectID)
	var body = []byte(fmt.Sprintf(`{"image_id": "%s", "operation": "publish"}`, image.ID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("failed to create request to publish image: %w", err)
	}

	addHeaders(req, apiKey)

	res, err := httpClient.Do(req)
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
