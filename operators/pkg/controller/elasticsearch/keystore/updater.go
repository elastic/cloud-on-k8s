// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/utils/fs"
	"k8s.io/client-go/util/workqueue"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	attemptReload           = "attempt-reload"
	dirWatcherPollingPeriod = 1 * time.Second
)

var (
	log = logf.Log.WithName("keystore-updater")
)

// Updater updates the Elasticsearch keystore by watching a local directory corresponding to a Kubernetes secret.
type Updater struct {
	config      Config
	reloadQueue workqueue.DelayingInterface
	status      Status
	lock        sync.RWMutex
	esClient    EsClient
	keystore    Keystore
}

// NewUpdater returns a new keystore updater.
func NewUpdater(cfg Config, esClient EsClient, keystore Keystore) (*Updater, error) {
	status := Status{notInitializedState, "Keystore updater created", time.Now()}

	return &Updater{
		config:      cfg,
		reloadQueue: workqueue.NewDelayingQueue(),
		status:      status,
		lock:        sync.RWMutex{},
		esClient:    esClient,
		keystore:    keystore,
	}, nil
}

// Status returns the keystore updater status
func (u *Updater) Status() (Status, error) {
	u.lock.RLock()
	defer u.lock.RUnlock()
	return u.status, nil
}

// updateStatus updates the Keystore updater status
func (u *Updater) updateStatus(s State, msg string, err error) {
	u.lock.Lock()
	defer u.lock.Unlock()
	reason := msg
	if err != nil {
		reason = fmt.Sprintf("%s: %s", reason, err.Error())
	}
	u.status = Status{s, reason, time.Now()}
}

// Start updates the keystore once and then starts a watcher on source dir to update again on file changes.
func (u *Updater) Start() {
	u.updateStatus(waitingState, "Waiting for Elasticsearch to be ready", nil)
	u.esClient.WaitForEsReady()

	if u.config.ReloadCredentials {
		go u.coalescingRetry()
	}

	go u.watchForUpdate()
}

func (u *Updater) watchForUpdate() {
	// on each filesystem event for config.SourceDir, update the keystore
	onEvent := func(files fs.FilesCRC) (stop bool, e error) {
		log.Info("On event")
		err, msg := u.updateKeystore()
		if err != nil {
			log.Error(err, "Cannot update keystore", "msg", msg)
			u.updateStatus(failedState, msg, err)
		} else {
			u.updateStatus(runningState, keystoreUpdatedReason, nil)
		}
		return false, nil // run forever
	}

	log.Info("Watch for update")
	watcher, err := fs.DirectoryWatcher(context.Background(), u.config.SecretsSourceDir, onEvent, dirWatcherPollingPeriod)
	if err != nil {
		msg := "Cannot watch filesystem"
		log.Error(err, msg, "path", u.config.SecretsSourceDir)
		u.updateStatus(failedState, msg, err)
		return
	}
	// execute at least once with the initial fs content
	err, msg := u.updateKeystore()
	if err != nil {
		log.Error(err, "Cannot update keystore", "msg", msg)
		u.updateStatus(failedState, msg, err)
	} else {
		u.updateStatus(runningState, keystoreUpdatedReason, err)
	}

	// then run on files change
	if err := watcher.Run(); err != nil {
		msg := "Cannot watch filesystem"
		log.Error(err, msg, "path", u.config.SecretsSourceDir)
		u.updateStatus(failedState, msg, err)
	}
}

// coalescingRetry attempts to reload the keystore coalescing subsequent requests into one when retrying.
func (u *Updater) coalescingRetry() {
	var item interface{}
	shutdown := false
	for !shutdown {
		log.Info("Wait for reloading secure settings")
		item, shutdown = u.reloadQueue.Get()

		log.Info("Reloading secure settings")
		err := u.esClient.ReloadSecureSettings()
		if err != nil {
			msg := "Failed to reload secure settings"
			log.Error(err, msg+". Continuing.")
			u.updateStatus(failedState, msg, err)
			u.reloadQueue.AddAfter(item, 5*time.Second) // TODO exp. backoff w/ jitter
		} else {
			u.updateStatus(runningState, secureSettingsReloadedReason, nil)
			log.Info(secureSettingsReloadedReason)
		}
		u.reloadQueue.Done(item)
	}
}

// updateKeystore reconciles the source directory with the Elasticsearch keystore by recreating the
// keystore and adding a setting for each file in the source directory.
func (u *Updater) updateKeystore() (error, string) {
	// TODO: can we do that to a running cluster?
	ok, err := u.keystore.Delete()
	if err != nil {
		return err, "could not delete keystore file"
	}
	if ok {
		log.Info("Deleted keystore", "keystore-path", u.config.KeystorePath)
	}

	log.Info("Creating keystore", "keystore-path", u.config.KeystorePath)
	err = u.keystore.Create()
	if err != nil {
		return err, "could not create new keystore"
	}

	fileInfos, err := ioutil.ReadDir(u.config.SecretsSourceDir)
	if err != nil {
		return err, "could not read settings source directory"
	}

	for _, file := range fileInfos {
		if strings.HasPrefix(file.Name(), ".") {
			log.Info(fmt.Sprintf("Ignoring %s", file.Name()))
			continue
		}

		log.Info("Adding setting to keystore", "file", file.Name())
		err = u.keystore.AddSetting(file.Name())
		if err != nil {
			return err, fmt.Sprintf("could not add setting %s", file.Name())
		}
	}

	settings, err := u.keystore.ListSettings()
	if err != nil {
		return err, "error during listing keystore settings"
	}
	log.Info("Keystore updated", "settings", settings)

	if u.config.ReloadCredentials {
		u.reloadQueue.Add(attemptReload)
	}

	return nil, ""
}
