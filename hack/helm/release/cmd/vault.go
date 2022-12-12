package cmd

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/spf13/viper"
)

const (
	credentialsPath    = "/tmp/credentials.json"
	vaultSecretDataKey = "creds.json"
)

// readCredentialsFromVault will will read a secret from vault
// with the address at environment variable VAULT_ADDR, using vault
// token at environment variable VAULT_TOKEN, using configured secret
// path from viper key 'vault-secret', writing it to '/tmp/credentials.json',
// and setting the viper key 'credentials-file'.
func readCredentialsFromVault() error {
	client, err := api.NewClient(&api.Config{
		Address: os.Getenv("VAULT_ADDR"),
		HttpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	})

	if err != nil {
		return fmt.Errorf("failed to create vault client: %w", err)
	}

	client.SetToken(os.Getenv("VAULT_TOKEN"))

	vaultSecret := viper.GetString("vault-secret")
	secret, err := client.Logical().Read(vaultSecret)
	if err != nil {
		return fmt.Errorf("failed to read vault secret (%s): %w", vaultSecret, err)
	}

	if secret == nil {
		return fmt.Errorf("found no secret at location: %s", vaultSecret)
	}

	data, ok := secret.Data[vaultSecretDataKey]
	if !ok {
		return fmt.Errorf("key (creds.json) not found in vault in secret (%s)", vaultSecret)
	}

	stringData, ok := data.(string)
	if !ok {
		return fmt.Errorf("failed to convert secrets data to string: %T %#v", secret.Data[vaultSecretDataKey], secret.Data[vaultSecretDataKey])
	}

	if err := ioutil.WriteFile(credentialsPath, []byte(stringData), 0600); err != nil {
		return fmt.Errorf("while writing gcs credentials file read from vault: %w", err)
	}
	viper.Set("credentials-file", credentialsPath)

	return nil
}
