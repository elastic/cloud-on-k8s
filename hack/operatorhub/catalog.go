// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const (
	ImagesEndpoint = "https://catalog.redhat.com/api/containers/v1/projects/certification/id/%s/images"
)

type Images struct {
	Data []struct {
		DockerImageDigest string `json:"docker_image_digest"`
		Id                string `json:"_id"`
		CreationDate      string `json:"creation_date"`
	} `json:"data"`
}

// getImageDigest connects to the RedHat catalog API to get the certified operator image digest as it is exposed
// by the RedHat registry.
func getImageDigest(apiKey, projectId, version string) (string, error) {
	requestURL := fmt.Sprintf(ImagesEndpoint, projectId)

	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-KEY", apiKey)

	q := req.URL.Query()
	q.Add("filter", fmt.Sprintf("repositories.tags.name==%s;deleted==false", version))
	req.URL.RawQuery = q.Encode()

	client := http.Client{
		Timeout: 30 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request error %s: %s", requestURL, res.Status)
	}

	var images Images
	if err := json.NewDecoder(res.Body).Decode(&images); err != nil {
		return "", err
	}
	if len(images.Data) > 1 {
		fmt.Fprintf(os.Stderr, "\nid                       creation_date                    docker_image_digest\n")
		for _, image := range images.Data {
			fmt.Fprintf(os.Stderr, "%s %s %s\n", image.Id, image.CreationDate, image.DockerImageDigest)
		}
		return "", fmt.Errorf("found %d images with tag %s in RedHat catalog while only one is expected", len(images.Data), version)
	}
	if len(images.Data) == 0 {
		return "", fmt.Errorf("found %d images with tag %s in RedHat catalog, at least one is expected", len(images.Data), version)
	}
	imageDigest := images.Data[0].DockerImageDigest
	if imageDigest == "" {
		return "", fmt.Errorf("image digest for %s is empty", version)
	}
	return imageDigest, nil
}
