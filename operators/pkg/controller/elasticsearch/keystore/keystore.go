// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// StringSettingPrefix is used to prefix a string secure setting
	StringSettingPrefix string = "es.string."
	// FileSettingPrefix is used to prefix a file secure setting
	FileSettingPrefix string = "es.file."
)

// Keystore is used to manage settings stored in the Elasticsearch keystore.
type Keystore interface {
	// Create a new Elasticsearch keystore
	Create() error
	// Delete the Elasticsearch keystore
	Delete() (bool, error)
	// ListSettings lists the settings in the keystore
	ListSettings() (string, error)
	// AddSetting adds either a file or string setting to the keystore, depending on the file prefix
	AddSetting(filename string) error
	// AddFileSetting adds a file setting to the keystore
	AddFileSetting(filename string) error
	// AddFileSettings adds a string setting to the keystore from a file content
	AddStringSetting(filename string) error
}

// CmdRunner runs an exec.Cmd. It is mostly used as an abstraction for testing purpose.
type CmdRunner interface {
	Run(cmd *exec.Cmd) error
	Output(cmd *exec.Cmd) ([]byte, error)
}

// execCmdRunner is an implementation of CmdRunner that simply relies on the builtin exec.Cmd.
type execCmdRunner struct{}

func (e *execCmdRunner) Run(cmd *exec.Cmd) error {
	return cmd.Run()
}
func (e *execCmdRunner) Output(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

// keystore is the default Keystore implementation that relies on the elasticsearch-keystore binary.
type keystore struct {
	// binaryPath is the path of the elasticsearch-keystore binary
	binaryPath string
	// keystorePath is the path of the elasticsearch.keystore file used to store secure settings on disk
	keystorePath string
	// settingsPath is the path of the directory where the secure settings to store in the keystore live
	settingsPath string
	// cmdRunner executes the given cmd, can be overridden for testing purpose
	cmdRunner CmdRunner
}

func NewKeystore(cfg Config) Keystore {
	return keystore{
		binaryPath:   cfg.KeystoreBinary,
		keystorePath: cfg.KeystorePath,
		settingsPath: cfg.SecretsSourceDir,
		cmdRunner:    &execCmdRunner{},
	}
}

func (c keystore) Create() error {
	create := exec.Command(c.binaryPath, "create", "--silent")
	create.Dir = filepath.Dir(c.keystorePath)
	return create.Run()
}

func (c keystore) ListSettings() (string, error) {
	bytes, err := c.cmdRunner.Output(exec.Command(c.binaryPath, "list"))
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(`\r?\n`)
	settings := re.ReplaceAllString(string(bytes), " ")
	return settings, nil
}

// AddSetting adds either a file or string setting to the keystore, depending on the file prefix.
// It defaults to a string setting if no prefix is set.
func (c keystore) AddSetting(filename string) error {
	if strings.HasPrefix(filename, FileSettingPrefix) {
		return c.AddFileSetting(filename)
	}
	return c.AddStringSetting(filename)
}

func (c keystore) AddFileSetting(filename string) error {
	settingName := strings.TrimPrefix(filename, FileSettingPrefix)
	cmd := exec.Command(c.binaryPath, "add-file", settingName, path.Join(c.settingsPath, filename))
	return c.cmdRunner.Run(cmd)
}

func (c keystore) readSettingFileContent(filename string) ([]byte, error) {
	return ioutil.ReadFile(path.Join(c.settingsPath, filename))
}

func (c keystore) AddStringSetting(filename string) error {
	value, err := c.readSettingFileContent(filename)
	if err != nil {
		return err
	}
	settingName := strings.TrimPrefix(filename, StringSettingPrefix)
	cmd := exec.Command(c.binaryPath, "add", settingName)
	// pipe the file content into stdin to set the string setting value
	cmd.Stdin = bytes.NewBuffer(value)
	return c.cmdRunner.Run(cmd)
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
