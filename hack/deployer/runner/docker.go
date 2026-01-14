// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"errors"
	"os"
)

const defaultDockerSocket = "/var/run/docker.sock"

var homeDockerSocket = os.ExpandEnv("${HOME}/.docker/run/docker.sock")

func getDockerSocket() (string, error) {
	sck, err := followLink(defaultDockerSocket)
	if err == nil { // if *not* error, return the socket
		return sck, nil
	}

	hsc, hErr := followLink(homeDockerSocket)
	if hErr != nil {
		return "", errors.Join(err, hErr)
	}

	return hsc, nil
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
