// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package vault

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/hashicorp/vault/api"
)

const (
	vaultAddrEnvVar     = "VAULT_ADDR"
	vaultTokenEnvVar    = "VAULT_TOKEN"
	vaultRootPathEnvVar = "VAULT_ROOT_PATH"

	// buildLicensePubKeyPrefixEnvVar allows to prefix the field to retrieve a specific license public key secret
	buildLicensePubKeyPrefixEnvVar = "BUILD_LICENSE_PUBKEY"
	defaultVaultRootPath           = "secret/ci/elastic-cloud-on-k8s"

	// inMemory is the name of the file to use to not write it to disk and only keep it in memory
	inMemory = "in-memory"
)

// SecretFile maps a Vault Secret into a file that is optionally written to disk
type SecretFile struct {
	// Name is the name of the file to optionaly write the secret to disk. If the value is 'in-memory' the file is not written to disk.
	Name string
	// Path is the Vault path to read the secret
	Path string
	// FormatJSON indicates if the secret needs to be printed in JSON format. It can be only true if FieldResolver is not set.
	FormatJSON bool
	// FieldResolver is a function to get only a given field of the secret. It is optional and can be nil.
	// It can be only set if FormatJSON is false.
	FieldResolver func() string
	// Base64Encoded indicates if the secret needs to be decoded. It is only usable with FieldResolver is set.
	Base64Encoded bool

	// client is the client to connect to Vault.
	// Exposing it here allows you to inject a mock for testing.
	client vaultClient
}

// Read reads the file if it exists or else reads the Secret from Vault and
// writes it to the file on disk.
func (f SecretFile) Read() ([]byte, error) {
	if _, err := os.Stat(f.Name); err == nil {
		return os.ReadFile(f.Name)
	}

	bytes, err := f.readFromVault()
	if err != nil {
		return nil, err
	}

	if f.Name != inMemory {
		err = os.WriteFile(f.Name, bytes, 0600)
		if err != nil {
			return nil, err
		}
	}

	return bytes, nil
}

// LicensePubKeyPrefix prefixes the given field with the value of the build license public key environment variable.
func LicensePubKeyPrefix(field string) func() string {
	return func() string {
		prefix := os.Getenv(buildLicensePubKeyPrefixEnvVar)
		if prefix != "" {
			field = fmt.Sprintf("%s-%s", prefix, field)
		}
		return field
	}
}

func (f SecretFile) readFromVault() ([]byte, error) {
	if os.Getenv(vaultAddrEnvVar) == "" {
		return nil, fmt.Errorf("%s must be set", vaultAddrEnvVar)
	}
	if os.Getenv(vaultTokenEnvVar) == "" {
		return nil, fmt.Errorf("%s must be set", vaultTokenEnvVar)
	}

	log.Printf("Read secret %s from vault", f.Path)

	var secret *api.Secret
	if err := retry.Do(
		func() error {
			// use the client or create a new one
			var client = f.client
			if client == nil {
				c, err := api.NewClient(api.DefaultConfig())
				if err != nil {
					return err
				}
				client = c.Logical()
			}

			// prefix the path with the vault root path environment variable.
			rootPath := os.Getenv(vaultRootPathEnvVar)
			if rootPath == "" {
				rootPath = defaultVaultRootPath
			}

			// read the secret
			var err error
			secret, err = client.Read(fmt.Sprintf("%s/%s", rootPath, f.Path))
			if err != nil {
				return err
			}
			return nil
		},
		retry.Attempts(10),
		retry.Delay(1*time.Second),
		retry.OnRetry(func(n uint, err error) {
			log.Printf("retry #%d: %s\n", n, err)
		}),
	); err != nil {
		return nil, err
	}

	if secret == nil {
		return nil, fmt.Errorf("no data found at %s", f.Path)
	}

	// validate mutual exclusions
	if f.FieldResolver != nil && f.FormatJSON {
		return nil, fmt.Errorf("FieldResolver cannot be defined if FormatJSON is true")
	}
	if f.FieldResolver == nil && !f.FormatJSON {
		return nil, fmt.Errorf("FieldResolver must be defined if FormatJSON is false")
	}
	if f.FieldResolver == nil && f.Base64Encoded {
		return nil, fmt.Errorf("FieldResolver must be defined if Base64Encoded is true")
	}

	// encode to JSON and return
	if f.FormatJSON {
		return json.Marshal(secret.Data)
	}

	// get the field as string
	field := f.FieldResolver()
	var ok bool
	val, ok := secret.Data[field]
	if !ok {
		return nil, fmt.Errorf("field %s not found at %s", field, f.Path)
	}
	stringVal, ok := val.(string)
	if !ok {
		return nil, fmt.Errorf("secret %s is not a string", f.Path)
	}

	// decode from Base64 encoded and return
	if f.Base64Encoded {
		return base64.StdEncoding.DecodeString(stringVal)
	}

	// or return as is
	return []byte(stringVal), nil
}

type vaultClient interface {
	Read(path string) (*api.Secret, error)
}
