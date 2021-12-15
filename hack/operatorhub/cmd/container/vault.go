// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package container

import (
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"
)

func attemptVault() error {
	client, err := api.NewClient(&api.Config{
		Address: viper.GetString("vault-addr"),
		HttpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	})

	if err != nil {
		return fmt.Errorf("failed to create vault client: %w", err)
	}

	client.SetToken(viper.GetString("vault-token"))

	vaultSecret := viper.GetString("vault-secret")
	secret, err := client.Logical().Read(vaultSecret)
	if err != nil {
		return fmt.Errorf("failed to read vault secret (%s): %w", vaultSecret, err)
	}

	if secret == nil {
		return fmt.Errorf("found no secret at location: %s", vaultSecret)
	}

	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("failed to convert secrets data to map: %T %#v", secret.Data["data"], secret.Data["data"])
	}

	for _, k := range []string{"api-key", "redhat-connect-registry-key"} {
		value, ok := data[k]
		if ok {
			viper.Set(k, value)
		} else {
			pterm.Println(pterm.Yellow(fmt.Sprintf("key (%s) not found in vault (%s) in secret (%s)", k, viper.GetString("vault-addr"), vaultSecret)))
		}
	}

	return nil
}
