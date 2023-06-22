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
	"path/filepath"
	"time"
)

const (
	// InMemoryFile is the name of the file to use to not write it to disk and only keep it in memory
	InMemoryFile = "in-memory"

	// buildLicensePubKeyPrefixEnvVar allows to prefix the field to retrieve a specific license public key secret
	buildLicensePubKeyPrefixEnvVar = "BUILD_LICENSE_PUBKEY"

	retryTimeout  = 10 * time.Second
	retryInterval = 1 * time.Second
)

// SecretFile maps a Vault Secret into a file that is optionally written to disk.
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
}

// ReadFile reads the file if it exists or else reads the Secret from Vault and
// writes it to the file on disk.
func ReadFile(clientProvider ClientProvider, f SecretFile) ([]byte, error) {
	if _, err := os.Stat(f.Name); err == nil {
		return os.ReadFile(f.Name)
	}

	client, err := clientProvider()
	if err != nil {
		return nil, err
	}

	bytes, err := f.readFromVault(client)
	if err != nil {
		return nil, err
	}

	if f.Name != InMemoryFile {
		err = os.WriteFile(f.Name, bytes, 0600)
		if err != nil {
			return nil, err
		}
	}

	return bytes, nil
}

func (f SecretFile) readFromVault(c Client) ([]byte, error) {
	log.Printf("Read %s from vault", f.Path)

	secretPath := filepath.Join(rootPath(), f.Path)
	secret, err := read(c, secretPath)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("field %s not found at %s", field, secretPath)
	}
	stringVal, ok := val.(string)
	if !ok {
		return nil, fmt.Errorf("secret %s is not a string", secretPath)
	}

	// decode from Base64 encoded and return
	if f.Base64Encoded {
		return base64.StdEncoding.DecodeString(stringVal)
	}

	// or return as is
	return []byte(stringVal), nil
}

// LicensePubKeyPrefix is a specific field resolver that prefixes the given field with the value of the build license public key environment variable.
func LicensePubKeyPrefix(field string) func() string {
	return func() string {
		prefix := os.Getenv(buildLicensePubKeyPrefixEnvVar)
		if prefix != "" {
			field = fmt.Sprintf("%s-%s", prefix, field)
		}
		return field
	}
}
