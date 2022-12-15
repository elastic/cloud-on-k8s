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
	registryPasswordVaultSecretDataKey = "registry-password"
	projectIDVaultSecretDataKey        = "project-id"
	apiKeyVaultSecretDataKey           = "api-key"

	githubTokenVaultSecretDataKey    = "github-token"
	githubUsernameVaultSecretDataKey = "github-username"
	githubFullnameVaultSecretDataKey = "github-fullname"
	githubEmailVaultSecretDataKey    = "github-email"
)

var (
	redhatVaultSecretDataKeys = []string{registryPasswordVaultSecretDataKey, projectIDVaultSecretDataKey, apiKeyVaultSecretDataKey}
	githubVaultSecretDataKeys = []string{githubTokenVaultSecretDataKey, githubUsernameVaultSecretDataKey, githubFullnameVaultSecretDataKey, githubEmailVaultSecretDataKey}
)

func readSecretsFromVault() error {
	client, err := api.NewClient(&api.Config{
		Address: flags.VaultAddress,
		HttpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	})

	if err != nil {
		return fmt.Errorf("failed to create vault client: %w", err)
	}

	client.SetToken(flags.VaultToken)

	if err := readVaultSecrets(client, flags.RedhatVaultSecret, redhatVaultSecretDataKeys); err != nil {
		return err
	}

	if err := readVaultSecrets(client, flags.GithubVaultSecret, githubVaultSecretDataKeys); err != nil {
		return err
	}

	return nil
}

func readVaultSecrets(client *api.Client, vaultSecretPath string, keys []string) error {
	secret, err := client.Logical().Read(vaultSecretPath)
	if err != nil {
		return fmt.Errorf("failed to read vault secret (%s): %w", vaultSecretPath, err)
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
			return fmt.Errorf("failed to convert secret data to string: %T %#v", secret.Data[key], secret.Data[key])
		}

		viper.Set(key, stringData)
	}
	return nil
}
