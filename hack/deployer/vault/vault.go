// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/vault/api"
)

type Client struct {
	client      *api.Client
	roleID      string
	secretID    string
	token       string
	clientToken string
	rootPath    string
}

type Info struct {
	Address     string `yaml:"address"`
	RoleId      string `yaml:"roleId"`   //nolint:revive
	SecretId    string `yaml:"secretId"` //nolint:revive
	Token       string `yaml:"token"`
	ClientToken string `yaml:"clientToken"`
	RootPath    string `yaml:"rootPath"`
}

func NewClient(info Info) (*Client, error) {
	config := api.DefaultConfig()
	if err := config.ReadEnvironment(); err != nil {
		return nil, err
	}
	if info.Address != "" {
		config.Address = info.Address
	}
	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	c := &Client{
		client:      client,
		roleID:      info.RoleId,
		secretID:    info.SecretId,
		token:       info.Token,
		clientToken: info.ClientToken,
		rootPath:    info.RootPath,
	}
	if err := c.auth(); err != nil {
		return nil, err
	}
	return c, nil
}

// auth fetches the auth token using approle (with role id and secret id) or github (with token)
func (v *Client) auth() error {
	if v.client.Token() != "" {
		return nil
	}

	var data map[string]interface{}
	var method string
	var err error

	var clientToken string

	switch {
	case v.token != "":
		method = "github"
		data = map[string]interface{}{"token": v.token}
	case v.roleID != "" && v.secretID != "":
		method = "approle"
		data = map[string]interface{}{"role_id": v.roleID, "secret_id": v.secretID}
	case v.clientToken != "":
		method = "clientToken"
		clientToken = v.clientToken
	default:
		clientToken, err = readCachedToken()
		if err != nil {
			return fmt.Errorf("while attempting to read cached vault auth %w", err)
		}
		if clientToken == "" {
			return fmt.Errorf("please export VAULT_ADDR and run `vault login` before running deployer")
		}
	}

	if clientToken == "" {
		resp, err := v.client.Logical().Write(fmt.Sprintf("auth/%s/login", method), data)
		if err != nil {
			return err
		}

		if resp.Auth == nil {
			return fmt.Errorf("no auth info in response")
		}

		clientToken = resp.Auth.ClientToken
	}

	v.client.SetToken(clientToken)

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
	if os.IsNotExist(err) {
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

// ReadIntoFile is a helper function used to read from Vault into file
func (v *Client) ReadIntoFile(fileName, secretPath, fieldName string) error {
	res, err := v.read(secretPath)
	if err != nil {
		return err
	}

	serviceAccount, ok := res.Data[fieldName]
	if !ok {
		return fmt.Errorf("field %s not found at %s", fieldName, secretPath)
	}

	stringServiceAccount, ok := serviceAccount.(string)
	if !ok {
		return fmt.Errorf("field %s at %s is not a string, that's unexpected", fieldName, secretPath)
	}

	return os.WriteFile(fileName, []byte(stringServiceAccount), 0600)
}

// Get fetches contents of a single field at a specified path in Vault
func (v *Client) Get(secretPath string, fieldName string) (string, error) {
	result, err := v.GetMany(secretPath, fieldName)
	if err != nil {
		return "", err
	}

	return result[0], nil
}

// GetMany fetches contents of multiple fields at a specified path in Vault. If error is nil, result slice
// will be of length len(fieldNames).
func (v *Client) GetMany(secretPath string, fieldNames ...string) ([]string, error) {
	secret, err := v.read(secretPath)
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
func (v *Client) read(relativeSecretPath string) (*api.Secret, error) {
	absoluteSecretPath := filepath.Join(v.rootPath, relativeSecretPath)
	secret, err := v.client.Logical().Read(absoluteSecretPath)
	if secret == nil {
		return nil, fmt.Errorf("no data found at %s", absoluteSecretPath)
	}
	return secret, err
}
