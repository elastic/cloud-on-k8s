// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"context"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/utils/fs"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const attemptReload = "attempt-reload"

var (
	name = "keystore-updater"
	log  = logf.Log.WithName(name)
)

// Updater updates the keystore
type Updater struct {
	config Config
}

// NewUpdater returns a new keystore updater.
func NewUpdater(cfg Config) *Updater {
	return &Updater{
		config: cfg,
	}
}

// Status returns the keystore status.
func (u Updater) Status() (bool, error) {
	// FIXME: to implement
	return true, nil
}

// Start updates the keystore once and then starts a watcher on source dir to update again on file changes.
func (u Updater) Start() {
	if u.config.ReloadCredentials {
		go u.coalescingRetry()
	}

	go u.watchForUpdate()
}

func (u Updater) watchForUpdate() {
	// on each filesystem event for config.SourceDir, update the keystore
	onEvent := func(files fs.FilesCRC) (stop bool, e error) {
		log.Info("On event")
		err, msg := u.updateKeystore()
		if err != nil {
			log.Error(err, "Cannot update keystore", "msg", msg)
		}
		return false, nil // run forever
	}

	log.Info("Watch for update")
	watcher, err := fs.DirectoryWatcher(context.Background(), u.config.SecretsSourceDir, onEvent, 1*time.Second)
	if err != nil {
		// FIXME: should we exit here?
		log.Error(err, "Cannot watch filesystem", "path", u.config.SecretsSourceDir)
		return
	}
	if err := watcher.Run(); err != nil {
		log.Error(err, "Cannot watch filesystem", "path", u.config.SecretsSourceDir)
	}
}

// coalescingRetry attempts to reload the keystore coalescing subsequent requests into one when retrying.
func (u Updater) coalescingRetry() {
	var item interface{}
	shutdown := false
	for !shutdown {
		log.Info("Wait for reloading credentials")
		item, shutdown = u.config.ReloadQueue.Get()

		err, msg := u.reloadCredentials()
		if err != nil {
			log.Error(err, msg+". Continuing.")
			u.config.ReloadQueue.AddAfter(item, 5*time.Second) // TODO exp. backoff w/ jitter
		} else {
			log.Info("Successfully reloaded credentials")
		}
		u.config.ReloadQueue.Done(item)
	}
}

// reloadCredentials tries to make an API call to the reload_secure_credentials API
// to reload reloadable settings after the keystore has been updated.
func (u Updater) reloadCredentials() (error, string) {
	log.Info("Reloading secure settings")
	caCerts, err := loadCerts(u.config.EsCACertsPath)
	if err != nil {
		return err, "cannot create Elasticsearch client with CA certs"
	}
	api := client.NewElasticsearchClient(nil, u.config.EsEndpoint, u.config.EsUser, u.config.EsVersion, caCerts)
	// TODO this is problematic as this call is supposed to happen only when all nodes have the updated
	// keystore which is something we cannot guarantee from this process. Also this call will be issued
	// on each node which is redundant and might be problematic as well.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return api.ReloadSecureSettings(ctx), "Error reloading credentials"
}

// loadCerts returns the certificates given a certificates path.
func loadCerts(caCertPath string) ([]*x509.Certificate, error) {
	bytes, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		return nil, err
	}
	return certificates.ParsePEMCerts(bytes)
}

// updateKeystore reconciles the source directory with Elasticsearch keystores by recreating the
// keystore and adding a setting for each file in the source directory.
func (u Updater) updateKeystore() (error, string) {
	// delete existing keystore (TODO can we do that to a running cluster?)
	_, err := os.Stat(u.config.KeystorePath)
	if !os.IsNotExist(err) {
		log.Info("Removing keystore", "keystore-path", u.config.KeystorePath)
		err := os.Remove(u.config.KeystorePath)
		if err != nil {
			return err, "could not delete keystore file"
		}
	}

	log.Info("Creating keystore", "keystore-path", u.config.KeystorePath)
	create := exec.Command(u.config.KeystoreBinary, "create", "--silent")
	create.Dir = filepath.Dir(u.config.KeystorePath)
	err = create.Run()
	if err != nil {
		return err, "could not create new keystore"
	}

	fileInfos, err := ioutil.ReadDir(u.config.SecretsSourceDir)
	if err != nil {
		return err, "could not read source directory"
	}

	for _, file := range fileInfos {
		if strings.HasPrefix(file.Name(), ".") {
			log.Info(fmt.Sprintf("Ignoring %s", file.Name()))
			continue
		}
		log.Info("Adding setting to keystore", "file", file.Name())
		add := exec.Command(u.config.KeystoreBinary, "add-file", file.Name(), path.Join(u.config.SecretsSourceDir, file.Name()))
		err := add.Run()
		if err != nil {
			return err, fmt.Sprintf("could not add setting %s", file.Name())
		}
	}

	list := exec.Command(u.config.KeystoreBinary, "list")
	bytes, err := list.Output()
	if err != nil {
		return err, "error during listing keystore settings"
	}

	re := regexp.MustCompile(`\r?\n`)
	input := re.ReplaceAllString(string(bytes), " ")
	log.Info("keystore updated", "settings", input)

	if u.config.ReloadCredentials {
		u.config.ReloadQueue.Add(attemptReload)
	}

	return nil, ""
}
