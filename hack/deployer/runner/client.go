// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const clientBaseImageName = "docker.elastic.co/eck-ci/deployer"

func ensureClientImage(driverID, clientVersion string, clientBuildDefDir string) (string, error) {
	if clientVersion == "" {
		return "", errors.New("clientVersion must not be empty")
	}

	dockerfilePath := filepath.Join(clientBuildDefDir, driverID)
	dockerfileName := filepath.Join(dockerfilePath, "Dockerfile")

	image, err := clientImageName(driverID, clientVersion, dockerfileName)
	if err != nil {
		return "", fmt.Errorf("while calculting docker image name %w", err)
	}

	if exists := checkImageExists(image); exists {
		return image, nil
	}

	if err = NewCommand(
		fmt.Sprintf("docker build --build-arg CLIENT_VERSION=%s -f %s -t %s %s",
			clientVersion, dockerfileName, image, dockerfilePath),
	).Run(); err != nil {
		return image, fmt.Errorf("while building client image %s: %w", image, err)
	}

	if err = dockerLogin(); err != nil {
		return image, fmt.Errorf("while logging into docker registry: %w", err)
	}

	err = NewCommand(fmt.Sprintf("docker push %s", image)).RunWithRetries(5, 1*time.Hour)
	return image, err
}

func checkImageExists(image string) bool {
	// short circuit if locally available e.g in local dev mode
	if output, err := NewCommand(fmt.Sprintf("docker images -q %s", image)).Output(); len(output) > 0 && err == nil {
		return true
	}

	// check registry
	imageExists, err := NewCommand(fmt.Sprintf("docker pull -q %s", image)).OutputContainsAny(image)
	return imageExists && err == nil
}

func clientImageName(driverID, clientVersion, dockerfileName string) (string, error) {
	// hash Dockerfile to trigger rebuild on content changes
	f, err := os.Open(dockerfileName)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New224()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	// including driver id and version directly image/tag for human benefit
	return fmt.Sprintf("%s-%s:%s-%.8x", clientBaseImageName, driverID, clientVersion, h.Sum(nil)), nil
}

func dockerLogin() error {
	registryEnv := ".registry.env"
	if _, err := os.Stat(registryEnv); os.IsNotExist(err) {
		// not attempting login when registry env file does not exist (typically outside of CI)
		return nil
	}
	return NewCommand(`docker login -u "$DOCKER_LOGIN" -p "$DOCKER_PASSWORD" docker.elastic.co 2> /dev/null`).
		WithVariablesFromFile(registryEnv).Run()
}
