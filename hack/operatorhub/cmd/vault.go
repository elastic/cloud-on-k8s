// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package root

import (
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/spf13/viper"

	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/flags"
)

const (
	// the following 3 constants are keys expected
	// within the vault secret map for the redhat
	// certification workflow.
	registryPasswordVaultSecretDataKey = "registry-password"
	projectIDVaultSecretDataKey        = "project-id"
	apiKeyVaultSecretDataKey           = "api-key"

	// the following 4 constants are keys expected
	// within the vault secret map for the the
	// creation of pull requests against community/certified
	// github repositories.
	githubTokenVaultSecretDataKey    = "github-token"
	githubUsernameVaultSecretDataKey = "github-username"
	githubFullnameVaultSecretDataKey = "github-fullname"
	githubEmailVaultSecretDataKey    = "github-email"
)

var (
	redhatVaultSecretDataKeys = []string{registryPasswordVaultSecretDataKey, projectIDVaultSecretDataKey, apiKeyVaultSecretDataKey}
	githubVaultSecretDataKeys = []string{githubTokenVaultSecretDataKey, githubUsernameVaultSecretDataKey, githubFullnameVaultSecretDataKey, githubEmailVaultSecretDataKey}
)

// readAllSecretsFromVault will generate a new vault client using the address, and token
// set previously using viper, and attempt to read both the "redhat-vault-secret"
// and "github-vault-secret" flags from vault, extracting the given map's keys, and
// setting the appropriate viper key.
func readAllSecretsFromVault() error {
	client, err := api.NewClient(&api.Config{
		Address: flags.VaultAddress,
		HttpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	})

	if err != nil {
		return fmt.Errorf("while creating vault client: %w", err)
	}

	client.SetToken(flags.VaultToken)

	if err := readVaultSecretAndSetViperKeys(client, flags.RedhatVaultSecret, redhatVaultSecretDataKeys); err != nil {
		return err
	}

	if err := readVaultSecretAndSetViperKeys(client, flags.GithubVaultSecret, githubVaultSecretDataKeys); err != nil {
		return err
	}

	return nil
}

// readVaultSecretAndSetViperKeys will read a vault secret at "vaultSecretPath" using the given vault
// client, and for every key in the slice of keys, the value at the secret map's key will be read, and
// used to set the viper key of the same name.
func readVaultSecretAndSetViperKeys(client *api.Client, vaultSecretPath string, keys []string) error {
	secret, err := client.Logical().Read(vaultSecretPath)
	if err != nil {
		return fmt.Errorf("while reading vault secret (%s): %w", vaultSecretPath, err)
	}

	if secret == nil {
		return fmt.Errorf("found no secret at location: %s", vaultSecretPath)
	}

	for _, key := range keys {
		data, ok := secret.Data[key]
		if !ok {
			return fmt.Errorf("key (%s) not found in vault in secret (%s)", key, vaultSecretPath)
		}
		stringData, ok := data.(string)
		if !ok {
			return fmt.Errorf("while converting secret data to string: %T %#v", secret.Data[key], secret.Data[key])
		}

		viper.Set(key, stringData)
	}
	return nil
}
