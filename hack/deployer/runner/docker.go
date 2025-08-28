// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"os"
	"runtime"
)

const defaultDockerSocket = "/var/run/docker.sock"

func getDockerSocket() (string, error) {
	_, err := os.Stat(defaultDockerSocket)
	if err != nil {
		if os.IsNotExist(err) {
			// If we are on macOS and the docker socket does not exist, fall back
			if runtime.GOOS == "darwin" {
				return "$HOME/.docker/run/docker.sock", nil
			}
		} else {
			// Handle other errors
			return "", err
		}
	}
	return defaultDockerSocket, nil
}
