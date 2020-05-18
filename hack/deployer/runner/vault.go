// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/hashicorp/vault/api"
)

type VaultClient struct {
	client      *api.Client
	roleID      string
	secretID    string
	token       string
	clientToken string
}

func NewClient(info VaultInfo) (*VaultClient, error) {
	// Timeout is set to avoid the issue described in https://github.com/hashicorp/vault/issues/6710
	client, err := api.NewClient(&api.Config{Address: info.Address, Timeout: 120 * time.Second})
	if err != nil {
		return nil, err
	}

	return &VaultClient{
		client:      client,
		roleID:      info.RoleId,
		secretID:    info.SecretId,
		token:       info.Token,
		clientToken: info.ClientToken,
	}, nil
}

// auth fetches the auth token using approle (with role id and secret id) or github (with token)
func (v *VaultClient) auth() error {
	if v.client.Token() != "" {
		return nil
	}

	var data map[string]interface{}
	var method string

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
		return fmt.Errorf("vault auth info not present")
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

// ReadIntoFile is a helper function used to read from Vault into file
func (v *VaultClient) ReadIntoFile(fileName, secretPath, fieldName string) error {
	if err := v.auth(); err != nil {
		return err
	}

	res, err := v.client.Logical().Read(secretPath)
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

	return ioutil.WriteFile(fileName, []byte(stringServiceAccount), 0600)
}

// Get fetches contents of a single field at a specified path in Vault
func (v *VaultClient) Get(secretPath string, fieldName string) (string, error) {
	result, err := v.GetMany(secretPath, fieldName)
	if err != nil {
		return "", err
	}

	return result[0], nil
}

// GetMany fetches contents of multiple fields at a specified path in Vault. If error is nil, result slice
// will be of length len(fieldNames).
func (v *VaultClient) GetMany(secretPath string, fieldNames ...string) ([]string, error) {
	if err := v.auth(); err != nil {
		return nil, err
	}

	secret, err := v.client.Logical().Read(secretPath)
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
