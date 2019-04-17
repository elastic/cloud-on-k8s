// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	timeToStartEs = 100 * time.Millisecond
)

type EsClientOkMock struct{}

func (c EsClientOkMock) ReloadSecureSettings() error { return nil }
func (c EsClientOkMock) WaitForEsReady()             { time.Sleep(timeToStartEs) }

type EsClientKoMock struct{}

func (c EsClientKoMock) ReloadSecureSettings() error { return errors.New("failed") }
func (c EsClientKoMock) WaitForEsReady()             { time.Sleep(timeToStartEs) }

type KeystoreOkMock struct{}

func (k KeystoreOkMock) Create() error                        { return nil }
func (k KeystoreOkMock) Delete() (bool, error)                { return true, nil }
func (k KeystoreOkMock) ListSettings() (string, error)        { return "", nil }
func (k KeystoreOkMock) AddFileSetting(filename string) error { return nil }

type KeystoreKoMock struct{}

func (k KeystoreKoMock) Create() error                        { return errors.New("failed") }
func (k KeystoreKoMock) Delete() (bool, error)                { return true, nil }
func (k KeystoreKoMock) ListSettings() (string, error)        { return "", nil }
func (k KeystoreKoMock) AddFileSetting(filename string) error { return nil }

func TestCoalescingRetry_Ok(t *testing.T) {
	updater, err := NewUpdater(Config{
		ReloadCredentials: true,
	}, EsClientOkMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	// Add an item in the queue
	assert.Equal(t, 0, updater.reloadQueue.Len())
	updater.reloadQueue.Add(attemptReload)
	assert.Equal(t, 1, updater.reloadQueue.Len())

	// Start coalescingRetry in background and wait 1s
	go updater.coalescingRetry()
	time.Sleep(1 * time.Second)

	// The queue should be empty
	assert.Equal(t, 0, updater.reloadQueue.Len())

	// Status is running and settings have been reloaded
	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(runningState), string(s.State))
	assert.Equal(t, secureSettingsReloadedReason, s.Reason)
}

func TestCoalescingRetry_Ko(t *testing.T) {
	updater, err := NewUpdater(Config{
		ReloadCredentials: true,
	}, EsClientKoMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	// Add an item in the queue
	updater.reloadQueue.Add(attemptReload)
	assert.Equal(t, 1, updater.reloadQueue.Len())

	// Start coalescingRetry in background and wait 1s
	go updater.coalescingRetry()
	time.Sleep(1 * time.Second)

	// The queue should be empty
	assert.Equal(t, 0, updater.reloadQueue.Len())

	// Status is failed
	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(failedState), string(s.State))
}

func TestWatchForUpdate(t *testing.T) {
	sourcePath, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(sourcePath)

	updater, err := NewUpdater(Config{
		SecretsSourceDir:  sourcePath,
		ReloadCredentials: true,
	}, EsClientOkMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	// Start watchForUpdate and wait at least one polling
	go updater.watchForUpdate()
	time.Sleep(dirWatcherPollingPeriod * 1)

	// Consume the queue manually
	assert.Equal(t, 1, updater.reloadQueue.Len())
	item, _ := updater.reloadQueue.Get()
	updater.reloadQueue.Done(item)
	assert.Equal(t, 0, updater.reloadQueue.Len())

	// Write a new settings to be stored in the keystore
	_ = ioutil.WriteFile(filepath.Join(sourcePath, "setting1"), []byte("secret1"), 0644)
	time.Sleep(dirWatcherPollingPeriod * 2)

	// The queue had to be filled
	assert.Equal(t, 1, updater.reloadQueue.Len())

	// Status is running and keystore is updated
	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(runningState), string(s.State))
	assert.Equal(t, string(keystoreUpdatedReason), string(s.Reason))
}

func TestStart_WaitingEs(t *testing.T) {
	sourcePath, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(sourcePath)

	updater, err := NewUpdater(Config{
		ReloadCredentials: true,
		SecretsSourceDir:  sourcePath,
	}, EsClientOkMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	go updater.Start()
	// Do not wait that ES is ready
	time.Sleep(timeToStartEs / 2)

	// Status is waiting
	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(waitingState), string(s.State))
}

func TestStart_UpdatedAtLeastOnce(t *testing.T) {
	sourcePath, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(sourcePath)

	updater, err := NewUpdater(Config{
		ReloadCredentials: false,
		SecretsSourceDir:  sourcePath,
	}, EsClientOkMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	go updater.Start()
	// Wait twice the time needed for ES to be ready
	time.Sleep(timeToStartEs * 2)

	// Status is running and keystore is updated
	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(runningState), string(s.State))
	assert.Equal(t, keystoreUpdatedReason, string(s.Reason))
}

func TestStart_Reload(t *testing.T) {
	sourcePath, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(sourcePath)

	updater, err := NewUpdater(Config{
		ReloadCredentials: true,
		SecretsSourceDir:  sourcePath,
	}, EsClientOkMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	go updater.Start()
	time.Sleep(timeToStartEs * 2)

	// Status is running and settings have been reloaded
	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(runningState), string(s.State))
	assert.Equal(t, secureSettingsReloadedReason, string(s.Reason))
}

func TestStart_ReloadAndWatchUpdate(t *testing.T) {
	sourcePath, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(sourcePath)

	updater, err := NewUpdater(Config{
		SecretsSourceDir:  sourcePath,
		ReloadCredentials: false,
	}, EsClientOkMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	go updater.Start()
	time.Sleep(timeToStartEs * 2)

	// Status is running and keystore is updated
	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(runningState), string(s.State))
	assert.Equal(t, keystoreUpdatedReason, string(s.Reason))

	// Write a setting and wait a bit to give time to the updater to watch it
	_ = ioutil.WriteFile(filepath.Join(sourcePath, "setting1"), []byte("secret1"), 0644)
	time.Sleep(dirWatcherPollingPeriod + 100*time.Millisecond)

	// Status is running and keystore is updated
	s, err = updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(runningState), string(s.State))
	assert.Equal(t, keystoreUpdatedReason, string(s.Reason))
}

func TestStart_ReloadFailure(t *testing.T) {
	updater, err := NewUpdater(Config{
		ReloadCredentials: true,
	}, EsClientKoMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	go updater.Start()
	time.Sleep(timeToStartEs * 2)

	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(failedState), string(s.State))
}

func TestStart_KeystoreUpdateFailure(t *testing.T) {
	sourcePath, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(sourcePath)

	updater, err := NewUpdater(Config{
		SecretsSourceDir:  sourcePath,
		ReloadCredentials: false,
	}, EsClientOkMock{}, KeystoreKoMock{})
	assert.NoError(t, err)

	go updater.Start()
	time.Sleep(timeToStartEs * 2)

	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(failedState), string(s.State))
}
