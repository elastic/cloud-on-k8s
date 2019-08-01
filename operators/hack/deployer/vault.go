// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import "io/ioutil"

const (
	vaultTokenName = "VAULT_TOKEN"
)

// ReadVaultIntoFile is a helper function used to read from Hashicorp Vault
func ReadVaultIntoFile(fileName, address, roleId, secretId, name string) error {
	vaultToken, err := NewCommand("vault write -address={{.Address}} -field=token auth/approle/login role_id={{.RoleId}} secret_id={{.SecretId}}").
		AsTemplate(map[string]interface{}{
			"Address":  address,
			"RoleId":   roleId,
			"SecretId": secretId,
		}).
		WithoutStreaming().
		Output()
	if err != nil {
		return err
	}

	serviceAccountKey, err := NewCommand("vault read -address={{.Address}} -field=service-account {{.Name}}").
		AsTemplate(map[string]interface{}{
			"Address": address,
			"Name":    name,
		}).
		WithVariable(vaultTokenName, vaultToken).
		WithoutStreaming().
		Output()
	if err != nil {
		return err
	}

	return ioutil.WriteFile(fileName, []byte(serviceAccountKey), 0644)
}
