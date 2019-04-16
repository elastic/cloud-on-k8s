// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
)

// Keystore is used to manage settings stored in the Elasticsearch keystore.
type Keystore interface {
	// Create a new Elasticsearch keystore
	Create() error
	// Delete the Elasticsearch keystore
	Delete() (bool, error)
	// ListSettings lists the settings in the keystore
	ListSettings() (string, error)
	// AddFileSettings adds a file settings to the keystore
	AddFileSettings(filename string) error
}

// keystore is the default Keystore implementation that relies on the elasticsearch-keystore binary.
type keystore struct {
	// binaryPath is the path of the elasticsearch-keystore binary
	binaryPath string
	// keystorePath is the path of the elasticsearch.keystore file used to store secure settings on disk
	keystorePath string
	// settingsPath is the path of the directory where the secure settings to store in the keystore live
	settingsPath string
}

func NewKeystoreCLI(cfg Config) Keystore {
	return keystore{
		binaryPath:   cfg.KeystoreBinary,
		keystorePath: cfg.KeystorePath,
		settingsPath: cfg.SecretsSourceDir,
	}
}

func (c keystore) Create() error {
	create := exec.Command(c.binaryPath, "create", "--silent")
	create.Dir = filepath.Dir(c.keystorePath)
	return create.Run()
}

func (c keystore) ListSettings() (string, error) {
	bytes, err := exec.Command(c.binaryPath, "list").Output()
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(`\r?\n`)
	settings := re.ReplaceAllString(string(bytes), " ")
	return settings, nil
}

func (c keystore) AddFileSettings(filename string) error {
	return exec.Command(c.binaryPath, "add-file", filename, path.Join(c.settingsPath, filename)).Run()
}

func (c keystore) Delete() (bool, error) {
	_, err := os.Stat(c.keystorePath)
	if !os.IsNotExist(err) {
		err := os.Remove(c.keystorePath)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}
