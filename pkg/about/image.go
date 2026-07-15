// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package about

import (
	"fmt"
	"os"
)

// GetOperatorImageFromEnv returns the container image of the running operator. It reads the OPERATOR_IMAGE
// environment variable, which is injected by the Helm chart and the OLM CSV at deploy time.
func GetOperatorImageFromEnv() (string, error) {
	img := os.Getenv("OPERATOR_IMAGE")
	if img == "" {
		return "", fmt.Errorf("OPERATOR_IMAGE environment variable is not set")
	}
	return img, nil
}
