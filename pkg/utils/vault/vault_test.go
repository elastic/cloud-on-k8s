// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package vault

import (
	"os"
	"testing"

	"github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/assert"
)

func Test_Read(t *testing.T) {
	f := SecretFile{
		Name:          "test.json",
		Path:          "test",
		FieldResolver: func() string { return "key" },
		client: newMockVaultClient(t,
			map[string]interface{}{
				"key": "42",
			},
		),
	}
	// garbage the file when the test is complete
	t.Cleanup(func() {
		os.Remove(f.Name)
	})

	// check that the file does NOT exists
	_, err := os.Stat(f.Name)
	assert.Error(t, err)

	// load the secret file
	bytes, err := f.Read()
	assert.NoError(t, err)
	assert.Equal(t, 1, readCount(f.client))
	assert.Equal(t, `42`, string(bytes))

	// check that the file exists
	_, err = os.Stat(f.Name)
	assert.NoError(t, err)

	// overwrite the file
	err = os.WriteFile(f.Name, []byte(`new_content`), 0600)
	assert.NoError(t, err)

	// load the file to checlk we read the new content and don't read in vault
	bytes, err = f.Read()
	assert.NoError(t, err)
	assert.Equal(t, 1, readCount(f.client))
	assert.Equal(t, `new_content`, string(bytes))

	// check that the file exists
	_, err = os.Stat(f.Name)
	assert.NoError(t, err)

	// delete the file
	err = os.Remove(f.Name)
	assert.NoError(t, err)

	// load again from vault to read the initial value
	bytes, err = f.Read()
	assert.NoError(t, err)
	assert.Equal(t, 2, readCount(f.client))
	assert.Equal(t, `42`, string(bytes))

	// delete the file
	err = os.Remove(f.Name)
	assert.NoError(t, err)

	// load in-memory
	f.Name = "in-memory"
	bytes, err = f.Read()
	assert.NoError(t, err)
	assert.Equal(t, 3, readCount(f.client))
	assert.Equal(t, `42`, string(bytes))

	// check that the file does not exist
	_, err = os.Stat(f.Name)
	assert.Error(t, err)
}

func Test_LicensePubKeyPrefix(t *testing.T) {
	f := SecretFile{
		Name:          "in-memory",
		Path:          "test",
		FieldResolver: LicensePubKeyPrefix("secret"),
		client: newMockVaultClient(t,
			map[string]interface{}{
				"secret":         "s3cr3t",
				"special-secret": "sp3c!@l",
			},
		),
	}

	// happy path
	bytes, err := f.Read()
	assert.NoError(t, err)
	assert.Equal(t, "s3cr3t", string(bytes))

	// happy path with the env var
	t.Setenv("BUILD_LICENSE_PUBKEY", "special")
	bytes, err = f.Read()
	assert.NoError(t, err)
	assert.Equal(t, "sp3c!@l", string(bytes))

	// error if the field is not found
	t.Setenv("BUILD_LICENSE_PUBKEY", "bad")
	_, err = f.Read()
	assert.Error(t, err)
}

func Test_SecretFile_Base64Encoded(t *testing.T) {
	f := SecretFile{
		Name:          "in-memory",
		Path:          "test",
		FieldResolver: func() string { return "f" },
		Base64Encoded: true,
		client: newMockVaultClient(t,
			map[string]interface{}{
				"f": "eyJ5b3BsYSI6ImJvdW0ifQ==",
			},
		),
	}

	// happy path: decode
	bytes, err := f.Read()
	assert.NoError(t, err)
	assert.Equal(t, `{"yopla":"boum"}`, string(bytes))

	// happy path: not decode
	f.Base64Encoded = false
	bytes, err = f.Read()
	assert.NoError(t, err)
	assert.Equal(t, `eyJ5b3BsYSI6ImJvdW0ifQ==`, string(bytes))

	// error if format JSON is set
	f.FormatJSON = true
	_, err = f.Read()
	assert.Error(t, err)
	// rollback
	f.FormatJSON = false

	// error if FieldResolver is not set
	f.FieldResolver = nil
	_, err = f.Read()
	assert.Error(t, err)
	// rollback
	f.FieldResolver = func() string { return "f" }

	// error if the secret is not in base64
	f.Base64Encoded = true
	f.client = newMockVaultClient(t,
		map[string]interface{}{
			"f": "notbase64",
		},
	)
	_, err = f.Read()
	assert.Error(t, err)
}

func Test_SecretFile_FormatJson(t *testing.T) {
	f := SecretFile{
		Name:       "in-memory",
		Path:       "test",
		FormatJSON: true,
		client: newMockVaultClient(t,
			map[string]interface{}{
				"a": "1",
				"b": "2",
			},
		),
	}

	// happy path
	bytes, err := f.Read()
	assert.NoError(t, err)
	assert.Equal(t, `{"a":"1","b":"2"}`, string(bytes))

	// error if FieldResolver is set
	f.FieldResolver = func() string { return "a" }
	_, err = f.Read()
	assert.Error(t, err)
}

type mockVaultClient struct {
	data      map[string]interface{}
	readCount int
}

func newMockVaultClient(t *testing.T, data map[string]interface{}) vaultClient {
	t.Helper()
	t.Setenv("VAULT_TOKEN", "0")
	t.Setenv("VAULT_ADDR", "secr.ets")
	return &mockVaultClient{
		data:      data,
		readCount: 0,
	}
}

func (c *mockVaultClient) Read(path string) (*api.Secret, error) {
	c.readCount++
	return &api.Secret{Data: c.data}, nil
}

func readCount(c vaultClient) int {
	m, _ := c.(*mockVaultClient)
	return m.readCount
}
