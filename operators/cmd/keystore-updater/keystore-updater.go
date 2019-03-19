// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"context"
	"crypto/x509"
	"fmt"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/utils/fs"
	"github.com/go-logr/logr"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const attemptReload = "attempt-reload"

type KeystoreUpdater struct {
	logger logr.Logger
	cfg    Config
}

func NewKeystoreUpdater(logger logr.Logger, cfg Config) *KeystoreUpdater {
	return &KeystoreUpdater{
		logger: logger,
		cfg:    cfg,
	}
}

// Run updates the keystore once and then starts a watcher on source dir to update again on file changes.
func (u KeystoreUpdater) Run() {
	if u.cfg.ReloadCredentials {
		go u.coalescingRetry()
	}

	u.watchForUpdate()
}

func (u KeystoreUpdater) watchForUpdate() {
	// on each filesystem event for config.SourceDir, update the keystore
	onEvent := func(files fs.FilesCRC) (stop bool, e error) {
		u.logger.Info("On event")
		err, msg := u.updateKeystore()
		if err != nil {
			u.logger.Error(err, "Cannot update keystore", "msg", msg)
		}
		return false, nil // run forever
	}

	u.logger.Info("Watch for update")
	watcher, err := fs.DirectoryWatcher(context.Background(), u.cfg.SourceDir, onEvent, 1*time.Second)
	if err != nil {
		// FIXME: should we exit here?
		u.logger.Error(err, "Cannot watch filesystem", "path", u.cfg.SourceDir)
		return
	}
	if err := watcher.Run(); err != nil {
		u.logger.Error(err, "Cannot watch filesystem", "path", u.cfg.SourceDir)
	}
}

// coalescingRetry attempts to reload the keystore coalescing subsequent requests into one when retrying.
func (u KeystoreUpdater) coalescingRetry() {
	u.logger.Info("Start coalescingRetry")
	var item interface{}
	shutdown := false
	for !shutdown {
		u.logger.Info("Wait for an item in the queue")
		item, shutdown = u.cfg.ReloadQueue.Get()

		u.logger.Info("reloadCredentials")
		err, msg := u.reloadCredentials()
		if err != nil {
			u.logger.Error(err, msg+". Continuing.")
			u.cfg.ReloadQueue.AddAfter(item, 5*time.Second) // TODO exp. backoff w/ jitter
		} else {
			u.logger.Info("Successfully reloaded credentials")
		}
		u.cfg.ReloadQueue.Done(item)
	}
}

// reloadCredentials tries to make an API call to the reload_secure_credentials API
// to reload reloadable settings after the keystore has been updated.
func (u KeystoreUpdater) reloadCredentials() (error, string) {
	caCerts, err := loadCerts(u.cfg.CACertsPath)
	if err != nil {
		return err, "cannot create Elasticsearch client with CA certs"
	}
	api := client.NewElasticsearchClient(nil, u.cfg.Endpoint, u.cfg.User, caCerts)
	// TODO this is problematic as this call is supposed to happen only when all nodes have the updated
	// keystore which is something we cannot guarantee from this process. Also this call will be issued
	// on each node which is redundant and might be problematic as well.
	u.logger.Info("ReloadSecureSettings")
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	return api.ReloadSecureSettings(ctx), "Error reloading credentials"
}

func loadCerts(caCertPath string) ([]*x509.Certificate, error) {
	bytes, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		return nil, err
	}
	return certificates.ParsePEMCerts(bytes)
}

// updateKeystore reconciles the source directory with Elasticsearch keystores by recreating the
// keystore and adding a setting for each file in the source directory.
func (u KeystoreUpdater) updateKeystore() (error, string) {
	// delete existing keystore (TODO can we do that to a running cluster?)
	_, err := os.Stat(u.cfg.KeystorePath)
	if !os.IsNotExist(err) {
		u.logger.Info("Removing keystore", "keystore-path", u.cfg.KeystorePath)
		err := os.Remove(u.cfg.KeystorePath)
		if err != nil {
			return err, "could not delete keystore file"
		}
	}

	u.logger.Info("Creating keystore", "keystore-path", u.cfg.KeystorePath)
	create := exec.Command(u.cfg.KeystoreBinary, "create", "--silent")
	create.Dir = filepath.Dir(u.cfg.KeystorePath)
	err = create.Run()
	if err != nil {
		return err, "could not create new keystore"
	}

	fileInfos, err := ioutil.ReadDir(u.cfg.SourceDir)
	if err != nil {
		return err, "could not read source directory"
	}

	for _, file := range fileInfos {
		if strings.HasPrefix(file.Name(), ".") {
			u.logger.Info(fmt.Sprintf("Ignoring %s", file.Name()))
			continue
		}
		u.logger.Info("Adding setting to keystore", "file", file.Name())
		add := exec.Command(u.cfg.KeystoreBinary, "add-file", file.Name(), path.Join(u.cfg.SourceDir, file.Name()))
		err := add.Run()
		if err != nil {
			return err, fmt.Sprintf("could not add setting %s", file.Name())
		}
	}

	list := exec.Command(u.cfg.KeystoreBinary, "list")
	bytes, err := list.Output()
	if err != nil {
		return err, "error during listing keystore settings"
	}

	re := regexp.MustCompile(`\r?\n`)
	input := re.ReplaceAllString(string(bytes), " ")
	u.logger.Info("keystore updated", "settings", input)

	if u.cfg.ReloadCredentials {
		u.cfg.ReloadQueue.Add(attemptReload)
	}

	return nil, ""
}
