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
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/docker/cli/cli/config"
	config_types "github.com/docker/cli/cli/config/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

const (
	eckOperatorFormat = "docker.elastic.co/eck/eck-operator-ubi8:%s"
	registryURL       = "quay.io"
)

var (
	errImageNotFound             = fmt.Errorf("image not found")
	catalogAPIURL                = "https://catalog.redhat.com/api/containers/v1"
	eckOperatorRegistryReference = "%s/redhat-isv-containers/%s:%s"
	registryUsername             = "redhat-isv-containers+%s-robot"
)

type PushConfig struct {
	DryRun              bool
	Force               bool
	HTTPClient          *http.Client
	ProjectID           string
	RedhatCatalogAPIKey string
	RegistryPassword    string
	Tag                 string
}

// PublishConfig is the configuration for the publish command
type PublishConfig struct {
	DryRun              bool
	Force               bool
	HTTPClient          *http.Client
	ProjectID           string
	RedhatCatalogAPIKey string
	RegistryPassword    string
	Tag                 string
	ImageScanTimeout    time.Duration
}

// PushImage will push an image to the redhat catalog if it is determined
// that the image doesn't already exist.  If 'Force' option is used, the
// image will be pushed regardless if it is already exists.
func PushImage(c PushConfig) error {
	if c.HTTPClient == nil {
		c.HTTPClient = defaultHTTPClient()
	}

	log.Printf("Determining if image already exists in project ")
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
	} else {
		log.Println("✓")
	}
	if err = pushImageToProject(c); err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}
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

// ImageExistsInProject will determine whether the image exists in the catalog api
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
	if pruneDeletedImages(images) == nil {
		log.Println("ignoring existing deleted image")
		return false, nil
	}
	return true, nil
}

func LoginToRegistry(dockerConfigDir, projectID, password string) error {
	cf, err := config.Load(dockerConfigDir)
	if err != nil {
		return fmt.Errorf("failed to load docker configuration: %w", err)
	}

	authConfig := config_types.AuthConfig{
		Username:      fmt.Sprintf(registryUsername, projectID),
		Password:      password,
		ServerAddress: registryURL,
	}

	creds := cf.GetCredentialsStore(registryURL)
	err = creds.Store(authConfig)
	if err != nil {
		return fmt.Errorf("failed to store auth configuration for %s: %w", registryURL, err)
	}

	err = cf.Save()
	if err != nil {
		return fmt.Errorf("failed to save docker configuration: %w", err)
	}
	return nil
}

func GetImageSHA(httpClient *http.Client, apiKey, projectID, tag string) (string, error) {
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
	image := pruneDeletedImages(images)
	if image == nil {
		return imageSHA, fmt.Errorf("couldn't find image with tag: %s", tag)
	}
	return image.DockerImageDigest, nil
}

func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
	}
}

func getImages(httpClient *http.Client, apiKey, projectID, tag string) ([]Image, error) {
	url := catalogAPIURL + "/projects/certification/id/" + projectID + "/images"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Add("filter", fmt.Sprintf("repositories.tags.name==%s", tag))
	req.URL.RawQuery = q.Encode()

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-API-KEY", apiKey)

	var res *http.Response
	if res, err = httpClient.Do(req); err != nil {
		return nil, fmt.Errorf("failed to request whether image exists: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode > 299 {
		if bodyBytes, err := ioutil.ReadAll(res.Body); err != nil {
			return nil, fmt.Errorf("failed to check whether image exists, body: %s, code: %d", string(bodyBytes), res.StatusCode)
		}
		return nil, fmt.Errorf("failed to check whether image exists, code: %d", res.StatusCode)
	}
	var bodyBytes []byte
	if bodyBytes, err = ioutil.ReadAll(res.Body); err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}
	response := GetImagesResponse{}
	if err = json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal body into valid response: %w", err)
	}
	return response.Images, nil
}

// pruneDeletedImages will ensure that images returned from the redhat catalog api
// are not simply artifacts (deleted images).  If the image(s) have been deleted, they
// do not return an architecture in the response, and simply return a subset of the
// top-level attributes.  This will return the first non-deleted image from the slice.
//
// TODO rename me.  This name doesn't make sense
func pruneDeletedImages(images []Image) *Image {
	for _, image := range images {
		if image.Architecture != nil {
			return &image
		}
	}
	return nil
}

func pushImageToProject(c PushConfig) error {
	// log.Printf("logging in to docker, and saving configuration: ")
	// err := LoginToRegistry(c.ProjectID, c.RegistryPassword)
	// if err != nil {
	// 	log.Println("ⅹ")
	// 	return err
	// }
	// log.Println("✓")

	if c.DryRun {
		log.Printf("not pushing image as dry-run is set.")
		return nil
	}

	username := fmt.Sprintf("redhat-isv-containers+%s-robot", c.ProjectID)
	formattedEckOperatorRedhatReference := fmt.Sprintf(eckOperatorRegistryReference, registryURL, c.ProjectID, c.Tag)
	imageToPull := fmt.Sprintf(eckOperatorFormat, c.Tag)

	log.Printf("pulling image (%s) in preparation to push (%s) to quay.io registry: ", imageToPull, formattedEckOperatorRedhatReference)
	image, err := crane.Pull(imageToPull, crane.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("while pulling (%s): %w", imageToPull, err)
	}
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
		return fmt.Errorf("while pushing %s: %s", formattedEckOperatorRedhatReference, err)
	}
	log.Println("✓")
	return nil
}

func publishImageInProject(httpClient *http.Client, imageScanTimeout time.Duration, apiKey, projectID, tag string, dryRun bool) error {
	ticker := time.NewTicker(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), imageScanTimeout)
	defer cancel()

	log.Printf("\nwaiting for image to complete scan process... ")
	image, done, err := isImageScanned(httpClient, imageScanTimeout, apiKey, projectID, tag)
	if err != nil {
		return err
	}
	if done {
		return doPublish(image, httpClient, apiKey, projectID, tag)
	}

	for {
		select {
		case <-ticker.C:
			image, done, err := isImageScanned(httpClient, imageScanTimeout, apiKey, projectID, tag)
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

func isImageScanned(httpClient *http.Client, imageScanTimeout time.Duration, apiKey, projectID, tag string) (image *Image, done bool, err error) {
	images, err := getImages(httpClient, apiKey, projectID, tag)
	if err != nil {
		log.Println(fmt.Sprintf("failed to find image in redhat catlog api, retrying: %s", err))
		return nil, false, nil
	}
	if len(images) == 0 {
		return nil, false, nil
	}
	image = pruneDeletedImages(images)
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

func doPublish(image *Image, httpClient *http.Client, apiKey, projectID, tag string) error {
	log.Printf("publishing image (%s), tag %s: ", image.ID, tag)
	// ensureHTTPClient(&c)
	url := fmt.Sprintf("%s/projects/certification/id/%s/requests/tags", catalogAPIURL, projectID)
	var body = []byte(fmt.Sprintf(`{"image_id": "%s", "tag": "%s"}`, image.ID, tag))
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("failed to create request to publish image: %w", err)
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-API-KEY", apiKey)
	// req.SetBasicAuth("Bearer", c.RedhatCatalogAPIKey)

	var res *http.Response
	if res, err = httpClient.Do(req); err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("failed request to publish image: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode > 299 {
		log.Println("ⅹ")
		if bodyBytes, err := ioutil.ReadAll(res.Body); err != nil {
			return fmt.Errorf("failed request to publish image, body: %s, code: %d", string(bodyBytes), res.StatusCode)
		}
		return fmt.Errorf("failed request to publish image, code: %d", res.StatusCode)
	}
	log.Println("✓")
	return nil
}
