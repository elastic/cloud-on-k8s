// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"os"
	"runtime"
)

const defaultDockerSocket = "/var/run/docker.sock"

var homeDockerSocket = os.ExpandEnv("${HOME}/.docker/run/docker.sock")

func getDockerSocket() (string, error) {
	if runtime.GOOS == "darwin" {
		sck, err := followLink(defaultDockerSocket)
		if err != nil {
			return followLink(homeDockerSocket)
		}

		return sck, nil
	}

	_, err := os.Stat(defaultDockerSocket)
	if err != nil {
		return "", err
	}

	return defaultDockerSocket, nil
}

func followLink(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return path, err
	}

	if isLink := info.Mode()&os.ModeSymlink != 0; !isLink {
		return path, nil
	}

	return os.Readlink(path)
}
