// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package env

import "os"

// SharedVolumeName name shared by CI container and Docker containers launched by deployer. This is the name of the volume
// valid outside of the CI Docker container, necessary to create other containers referencing the same volume.
// In local dev mode it is just the home dir as we are typically not running inside a container in the case.
func SharedVolumeName() string {
	if vol := os.Getenv("SHARED_VOLUME_NAME"); vol != "" {
		return vol
	}
	// use HOME for local dev mode
	return os.Getenv("HOME")
}
