// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestValidateConfig_InvalidSourceDir(t *testing.T) {
	_, err, msg := NewConfigFromFlags()
	assert.Error(t, err)
	assert.Equal(t, "source directory does not exist", msg)
}

func TestValidateConfig_InvalidBinaryPath(t *testing.T) {
	_ = os.Setenv(EnvSourceDir, "/tmp")

	err := BindEnvToFlags(&cobra.Command{})
	assert.NoError(t, err)

	_, err, msg := NewConfigFromFlags()
	assert.Error(t, err)
	assert.Equal(t, "keystore binary does not exist", msg)
}

func TestValidateConfig_InvalidVersion(t *testing.T) {
	_ = os.Setenv(EnvSourceDir, "/tmp")
	_ = os.Setenv(EnvKeystoreBinary, "/tmp")

	err := BindEnvToFlags(&cobra.Command{})
	assert.NoError(t, err)

	_, err, msg := NewConfigFromFlags()
	assert.Error(t, err)
	assert.Equal(t, "no or invalid version", msg)
}

func TestValidateConfig_InvalidUser(t *testing.T) {
	_ = os.Setenv(EnvSourceDir, "/tmp")
	_ = os.Setenv(EnvKeystoreBinary, "/tmp")
	_ = os.Setenv(EnvEsVersion, "7.1.0")

	err := BindEnvToFlags(&cobra.Command{})
	assert.NoError(t, err)

	_, err, msg := NewConfigFromFlags()
	assert.Error(t, err)
	assert.Equal(t, "invalid user", msg)
}

func TestValidateConfig_InvalidPasswordFile(t *testing.T) {
	_ = os.Setenv(EnvSourceDir, "/tmp")
	_ = os.Setenv(EnvKeystoreBinary, "/tmp")
	_ = os.Setenv(EnvEsVersion, "7.1.0")
	_ = os.Setenv(EnvEsUsername, "test")

	err := BindEnvToFlags(&cobra.Command{})
	assert.NoError(t, err)

	_, err, msg := NewConfigFromFlags()
	assert.Error(t, err)
	assert.Equal(t, "password file  could not be read", msg)
}

func TestValidateConfig_InvalidPassword(t *testing.T) {
	_ = os.Setenv(EnvSourceDir, "/tmp")
	_ = os.Setenv(EnvKeystoreBinary, "/tmp")
	_ = os.Setenv(EnvEsVersion, "7.1.0")
	_ = os.Setenv(EnvEsUsername, "test")
	_ = ioutil.WriteFile("/tmp/pwd", []byte(""), 0644)
	_ = os.Setenv(EnvEsPasswordFile, "/tmp/pwd")

	err := BindEnvToFlags(&cobra.Command{})
	assert.NoError(t, err)

	_, err, msg := NewConfigFromFlags()
	assert.Error(t, err)
	assert.Equal(t, "invalid password", msg)
}

func TestValidateConfig_Ok(t *testing.T) {
	_ = os.Setenv(EnvSourceDir, "/tmp")
	_ = os.Setenv(EnvKeystoreBinary, "/tmp")
	_ = os.Setenv(EnvEsVersion, "7.1.0")
	_ = os.Setenv(EnvEsUsername, "test")
	_ = ioutil.WriteFile("/tmp/pwd", []byte("x"), 0644)
	_ = os.Setenv(EnvEsPasswordFile, "/tmp/pwd")
	_ = ioutil.WriteFile("/tmp/cert", []byte("x"), 0644)
	_ = os.Setenv(EnvEsCaCertsPath, "/tmp/cert")

	err := BindEnvToFlags(&cobra.Command{})
	assert.NoError(t, err)

	_, err, msg := NewConfigFromFlags()
	assert.NoError(t, err)
	assert.Equal(t, "", msg)
}
