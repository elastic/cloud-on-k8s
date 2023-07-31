// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
)

const (
	addrEnvVar    = "VAULT_ADDR"
	tokenEnvVar   = "VAULT_TOKEN"
	roleIDEnvVar  = "VAULT_ROLE_ID"
	secretEnvVar  = "VAULT_SECRET_ID"
	ghTokenEnvVar = "GITHUB_TOKEN" //nolint:gosec
)

type Client interface {
	Read(path string) (*api.Secret, error)
}

type ClientProvider func() (Client, error)

func NewClientProvider() func() (Client, error) {
	var err error
	var client Client
	var once sync.Once
	return func() (Client, error) {
		once.Do(func() {
			client, err = NewClient()
		})
		return client, err
	}
}

func NewClient() (Client, error) {
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		return nil, err
	}

	if os.Getenv(addrEnvVar) == "" {
		return nil, fmt.Errorf("%s must be set", addrEnvVar)
	}

	if err := auth(client); err != nil {
		return nil, err
	}

	return client.Logical(), nil
}

// auth fetches the token using approle (with role id and secret id) or github (with token)
// if not already set through the environment or cached on disk.
func auth(c *api.Client) error {
	token := c.Token()

	// return if token is already set
	if token != "" {
		return nil
	}

	var data map[string]interface{}
	var method string

	roleID := os.Getenv(roleIDEnvVar)
	secretID := os.Getenv(secretEnvVar)
	ghToken := os.Getenv(ghTokenEnvVar)

	switch {
	case roleID != "" && secretID != "":
		method = "approle"
		data = map[string]interface{}{"role_id": roleID, "secret_id": secretID}
	case ghToken != "":
		method = "github"
		data = map[string]interface{}{"token": ghToken}
	default:
		var err error
		token, err = readCachedToken()
		if err != nil {
			return errors.Wrap(err, "while reading cached token")
		}
		if token == "" {
			return fmt.Errorf("set %s or %s/%s or %s or run `vault login`", tokenEnvVar, roleIDEnvVar, secretEnvVar, ghTokenEnvVar)
		}
	}

	if token == "" {
		resp, err := c.Logical().Write(fmt.Sprintf("auth/%s/login", method), data)
		if err != nil {
			return errors.Wrapf(err, "while logging into vault using method %s", method)
		}
		if resp.Auth == nil {
			return fmt.Errorf("while logging into vault: no auth info in response")
		}
		token = resp.Auth.ClientToken
	}

	c.SetToken(token)
	return nil
}

// readCachedToken attempts to read cached vault auth info from the users home directory. This aims mostly at the local
// dev mode and less at CI scenarios, so that users can log in with their vault credentials and deployer will pick up the
// auth token
func readCachedToken() (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, ".vault-token")
	_, err = os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil // no cached token present
	}
	if err != nil {
		return "", err
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytes)), nil
}
