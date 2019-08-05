// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"fmt"
	"io/ioutil"

	"github.com/hashicorp/vault/api"
)

const (
	serviceAccountKey = "service-account"
)

// ReadVaultIntoFile is a helper function used to read from Hashicorp Vault
func ReadVaultIntoFile(fileName, address, roleId, secretId, secretPath string) error {
	client, err := api.NewClient(&api.Config{Address: address})
	if err != nil {
		return err
	}

	// fetch the token
	data := map[string]interface{}{
		"role_id":   roleId,
		"secret_id": secretId,
	}
	resp, err := client.Logical().Write("auth/approle/login", data)
	if err != nil {
		return err
	}

	if resp.Auth == nil {
		return fmt.Errorf("no auth info in response")
	}

	client.SetToken(resp.Auth.ClientToken)

	// fetch the secret
	res, err := client.Logical().Read(secretPath)
	if err != nil {
		return err
	}

	serviceAccount, ok := res.Data[serviceAccountKey]
	if !ok {
		return fmt.Errorf("field %s not found at %s", serviceAccountKey, secretPath)
	}

	return ioutil.WriteFile(fileName, []byte(serviceAccount.(string)), 0644)
}
