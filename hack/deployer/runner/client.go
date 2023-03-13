// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/exec"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/vault"
)

const (
	dockerRegistry          = "docker.elastic.co"
	dockerRegistryVaultPath = "docker-registry-elastic"
	clientBaseImageName     = dockerRegistry + "/eck-ci/deployer"
)

func ensureClientImage(driverID string, vaultClient vault.Client, clientVersion string, clientBuildDefDir string) (string, error) {
	if clientVersion == "" {
		return "", errors.New("clientVersion must not be empty")
	}

	dockerfilePath := filepath.Join(clientBuildDefDir, driverID)
	dockerfileName := filepath.Join(dockerfilePath, "Dockerfile")

	image, err := clientImageName(driverID, clientVersion, dockerfileName)
	if err != nil {
		return "", fmt.Errorf("while calculating docker image name %w", err)
	}

	if exists := checkImageExists(image); exists {
		return image, nil
	}

	if err = exec.NewCommand(
		fmt.Sprintf("docker build --build-arg CLIENT_VERSION=%s -f %s -t %s %s",
			clientVersion, dockerfileName, image, dockerfilePath),
	).Run(); err != nil {
		return image, fmt.Errorf("while building client image %s: %w", image, err)
	}

	if err = dockerLogin(vaultClient); err != nil {
		return image, fmt.Errorf("while logging into docker registry: %w", err)
	}

	err = exec.NewCommand(fmt.Sprintf("docker push %s", image)).RunWithRetries(5, 1*time.Hour)
	return image, err
}

func checkImageExists(image string) bool {
	// short circuit if locally available e.g in local dev mode
	if output, err := exec.NewCommand(fmt.Sprintf("docker images -q %s", image)).Output(); len(output) > 0 && err == nil {
		return true
	}

	// check registry
	imageExists, err := exec.NewCommand(fmt.Sprintf("docker pull -q %s", image)).OutputContainsAny(image)
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

func dockerLogin(client vault.Client) error {
	// skip docker login in dev if registry exists in docker auth config file
	if os.Getenv("CI") != "true" { //nolint:nestif
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dockerConfig := filepath.Join(homeDir, ".docker", "config.json")
		if _, err := os.Stat(dockerConfig); err == nil {
			b, err := os.ReadFile(dockerConfig)
			if err != nil {
				return err
			}
			if bytes.Contains(b, []byte(dockerRegistry)) {
				log.Printf("Skip docker login as %s already exists in %s", dockerRegistry, dockerConfig)
				return nil
			}
		}
	}

	creds, err := vault.GetMany(client, dockerRegistryVaultPath, "username", "password")
	if err != nil {
		return err
	}

	return exec.NewCommand(fmt.Sprintf(`docker login -u "%s" -p "%s" %s 2> /dev/null`, creds[0], creds[1], dockerRegistry)).Run()
}
