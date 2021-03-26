// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

import (
	"errors"
	"fmt"
	"path/filepath"
)

const clientBaseImageName = "docker.elastic.co/eck-dev/deployer"

func ensureClientImage(driverID, clientVersion string) (string, error) {
	if clientVersion == "" {
		return "", errors.New("clientVersion must not be empty")
	}
	image := clientImageName(driverID, clientVersion) // todo: hash image!

	if exists := checkImageExists(image); exists {
		return image, nil
	}

	dockerfilePath := dockerfilePath(driverID)
	dockerfileName := filepath.Join(dockerfilePath, "Dockerfile")
	err := NewCommand(
		fmt.Sprintf("docker build --build-arg VERSION=%s -f %s -t %s %s",
			clientVersion, dockerfileName, image, dockerfilePath),
	).Run()
	if err != nil {
		return image, fmt.Errorf("while building client image %s: %w", image, err)
	}

	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		err = NewCommand(fmt.Sprintf("docker push %s", image)).Run()
		if err == nil {
			return image, nil
		}
	}
	return image, err
}

func checkImageExists(image string) bool {
	// short circuit if locally available e.g in local dev mode
	if output, err := NewCommand(fmt.Sprintf("docker images -q %s", image)).Output(); len(output) > 0 && err == nil {
		return true
	}

	// check registry
	if imageExists, err := NewCommand(fmt.Sprintf("docker pull -q %s", image)).OutputContainsAny("Downloading", "Extracting", "Verifying", "complete"); imageExists && err == nil {
		return true
	}
	return false
}

func clientImageName(driverID, clientVersion string) string {
	return fmt.Sprintf("%s:%s-%s", clientBaseImageName, driverID, clientVersion)
}

func dockerfilePath(driverID string) string {
	return filepath.Join(clientBuildDefDir, driverID)
}
