// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"fmt"
	"os/exec"
)

// checkDockerAvailable verifies that the Docker daemon is reachable.
func checkDockerAvailable() error {
	if err := exec.Command("docker", "info").Run(); err != nil {
		return fmt.Errorf("docker not available (is the daemon running?): %w", err)
	}
	return nil
}
