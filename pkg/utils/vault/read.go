// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package vault

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/vault/api"
)

const (
	// rootPathEnvVar contains the path to prefix all paths when reading secrets depending on vault backend used.
	rootPathEnvVar  = "VAULT_ROOT_PATH"
	defaultRootPath = "secret/ci/elastic-cloud-on-k8s"
)

// Get fetches contents of a single field at a specified path in Vault
func (c *Client) Get(secretPath string, fieldName string) (string, error) {
	result, err := c.GetMany(secretPath, fieldName)
	if err != nil {
		return "", err
	}

	return result[0], nil
}

// GetMany fetches contents of multiple fields at a specified path in Vault. If error is nil, result slice
// will be of length len(fieldNames).
func (c *Client) GetMany(secretPath string, fieldNames ...string) ([]string, error) {
	secret, err := c.read(secretPath)
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(fieldNames))
	for _, name := range fieldNames {
		val, ok := secret.Data[name]
		if !ok {
			return nil, fmt.Errorf("field %s not found at %s", name, secretPath)
		}

		stringVal, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("field %s at %s is not a string, that's unexpected", name, secretPath)
		}

		result = append(result, stringVal)
	}

	return result, nil
}

// read reads data from Vault at the given relative path appended to the root path configured at the client level.
// An error is returned if no data is found.
func (c *Client) read(relativeSecretPath string) (*api.Secret, error) {
	absoluteSecretPath := filepath.Join(rootPath(), relativeSecretPath)
	secret, err := c.Logical().Read(absoluteSecretPath)
	if secret == nil {
		return nil, fmt.Errorf("no data found at %s", absoluteSecretPath)
	}
	return secret, err
}

func rootPath() string {
	rootPath := os.Getenv(rootPathEnvVar)
	if rootPath == "" {
		return defaultRootPath
	}
	return rootPath
}
