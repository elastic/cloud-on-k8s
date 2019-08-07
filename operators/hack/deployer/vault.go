// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"fmt"
	"io/ioutil"

	"github.com/hashicorp/vault/api"
)

type Client struct {
	client   *api.Client
	roleId   string
	secretId string
	token    string
}

func NewClient(info VaultInfo) (*Client, error) {
	client, err := api.NewClient(&api.Config{Address: info.Address})
	if err != nil {
		return nil, err
	}

	return &Client{
		client:   client,
		roleId:   info.RoleId,
		secretId: info.SecretId,
		token:    info.Token,
	}, nil
}

// auth fetches the auth token using approle (with role id and secret id) or github (with token)
func (c *Client) auth() error {
	if c.client.Token() != "" {
		return nil
	}

	var data map[string]interface{}
	var method string

	if c.token != "" {
		method = "github"
		data = map[string]interface{}{"token": c.token}
	} else if c.roleId != "" && c.secretId != "" {
		method = "approle"
		data = map[string]interface{}{"role_id": c.roleId, "secret_id": c.secretId}
	} else {
		return fmt.Errorf("vault auth info not present")
	}

	resp, err := c.client.Logical().Write(fmt.Sprintf("auth/%s/login", method), data)
	if err != nil {
		return err
	}

	if resp.Auth == nil {
		return fmt.Errorf("no auth info in response")
	}

	c.client.SetToken(resp.Auth.ClientToken)

	return nil
}

// ReadIntoFile is a helper function used to read from Vault into file
func (c *Client) ReadIntoFile(fileName, secretPath, fieldName string) error {
	if err := c.auth(); err != nil {
		return err
	}

	res, err := c.client.Logical().Read(secretPath)
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

	return ioutil.WriteFile(fileName, []byte(stringServiceAccount), 0644)
}

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
	if err := c.auth(); err != nil {
		return nil, err
	}

	secret, err := c.client.Logical().Read(secretPath)
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
