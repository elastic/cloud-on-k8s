// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"os"
	"os/exec"
	"strings"
)

const defaultDockerSocket = "/var/run/docker.sock"

func getDockerSocket() (string, error) {
	// Ask the Docker CLI for the active context endpoint, which covers Docker Desktop,
	// Colima, Rancher Desktop, etc. without hardcoding runtime-specific paths.
	out, err := exec.Command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output()
	if err == nil {
		// Strip the unix:// scheme to get the raw path.
		path := strings.TrimPrefix(strings.TrimSpace(string(out)), "unix://")
		if path != "" {
			return followLink(path)
		}
	}
	// Fall back to the default socket location (typical in CI and Linux environments).
	return followLink(defaultDockerSocket)
}

func followLink(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return path, err
	}

	// if the file is not link, return the path.
	if info.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}

	return os.Readlink(path)
}
