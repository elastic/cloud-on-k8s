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

type ClientOkMock struct{}

func (c ClientOkMock) ReloadSecureSettings() error { return nil }
func (c ClientOkMock) WaitForEsReady()             { time.Sleep(timeToStartEs) }

type ClientKoMock struct{}

func (c ClientKoMock) ReloadSecureSettings() error { return errors.New("failed") }
func (c ClientKoMock) WaitForEsReady()             { time.Sleep(timeToStartEs) }

type KeystoreOkMock struct{}

func (k KeystoreOkMock) Create() error                         { return nil }
func (k KeystoreOkMock) Delete() (bool, error)                 { return true, nil }
func (k KeystoreOkMock) ListSettings() (string, error)         { return "", nil }
func (k KeystoreOkMock) AddFileSettings(filename string) error { return nil }

type KeystoreKoMock struct{}

func (k KeystoreKoMock) Create() error                         { return errors.New("failed") }
func (k KeystoreKoMock) Delete() (bool, error)                 { return true, nil }
func (k KeystoreKoMock) ListSettings() (string, error)         { return "", nil }
func (k KeystoreKoMock) AddFileSettings(filename string) error { return nil }

func TestCoalescingRetry_Ok(t *testing.T) {
	updater, err := NewUpdater(Config{
		ReloadCredentials: true,
	}, ClientOkMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	updater.reloadQueue.Add(attemptReload)
	assert.Equal(t, 1, updater.reloadQueue.Len())

	go updater.coalescingRetry()
	time.Sleep(1 * time.Second)

	assert.Equal(t, 0, updater.reloadQueue.Len())

	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(runningState), string(s.State))
}

func TestCoalescingRetry_Ko(t *testing.T) {
	updater, err := NewUpdater(Config{
		ReloadCredentials: true,
	}, ClientKoMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	// Add an item in the queue
	updater.reloadQueue.Add(attemptReload)
	assert.Equal(t, 1, updater.reloadQueue.Len())

	// Start coalescingRetry in background and wait 1s
	go updater.coalescingRetry()
	time.Sleep(1 * time.Second)

	// The queue had to be filled
	assert.Equal(t, 0, updater.reloadQueue.Len())

	// Status is KO
	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(failedState), string(s.State))
}

func TestWatchForUpdate(t *testing.T) {
	sourcePath, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(sourcePath) }()

	updater, err := NewUpdater(Config{
		SecretsSourceDir:  sourcePath,
		ReloadCredentials: true,
	}, ClientOkMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	// Starts watchForUpdate and wait at least one polling
	go updater.watchForUpdate()
	time.Sleep(dirWatcherPollingPeriod * 1)

	assert.Equal(t, 1, updater.reloadQueue.Len())

	// consume the queue manually
	item, _ := updater.reloadQueue.Get()
	updater.reloadQueue.Done(item)
	assert.Equal(t, 0, updater.reloadQueue.Len())

	// write a new settings to store in the keystore
	_ = ioutil.WriteFile(filepath.Join(sourcePath, "setting1"), []byte("secret1"), 0644)
	time.Sleep(dirWatcherPollingPeriod * 2)

	// the queue had to be filled
	assert.Equal(t, 1, updater.reloadQueue.Len())

	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(runningState), string(s.State))
	assert.Equal(t, string(keystoreUpdatedReason), string(s.Reason))
}

func TestStart_WaitingEs(t *testing.T) {
	sourcePath, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(sourcePath) }()

	updater, err := NewUpdater(Config{
		ReloadCredentials: true,
		SecretsSourceDir:  sourcePath,
	}, ClientOkMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	go updater.Start()
	time.Sleep(timeToStartEs / 2)

	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(waitingState), string(s.State))
}

func TestStart_UpdateAtLeastOnce(t *testing.T) {
	sourcePath, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(sourcePath) }()

	updater, err := NewUpdater(Config{
		ReloadCredentials: false,
		SecretsSourceDir:  sourcePath,
	}, ClientOkMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	go updater.Start()
	time.Sleep(timeToStartEs * 2)

	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(runningState), string(s.State))
	assert.Equal(t, "Keystore updated", string(s.Reason))
}

func TestStart_Reload(t *testing.T) {
	sourcePath, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(sourcePath) }()

	updater, err := NewUpdater(Config{
		ReloadCredentials: true,
		SecretsSourceDir:  sourcePath,
	}, ClientOkMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	go updater.Start()
	time.Sleep(timeToStartEs * 2)

	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(runningState), string(s.State))
	assert.Equal(t, "Successfully reloaded secure settings", string(s.Reason))
}

func TestStart_ReloadAndWatchUpdate(t *testing.T) {
	sourcePath, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(sourcePath) }()

	updater, err := NewUpdater(Config{
		SecretsSourceDir:  sourcePath,
		ReloadCredentials: false,
	}, ClientOkMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	go updater.Start()
	time.Sleep(timeToStartEs * 2)

	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(runningState), string(s.State))
	assert.Equal(t, "Keystore updated", string(s.Reason))

	// Write a setting and wait a bit to give time to the updater to watch it
	_ = ioutil.WriteFile(filepath.Join(sourcePath, "setting1"), []byte("secret1"), 0644)
	time.Sleep(dirWatcherPollingPeriod + 100*time.Millisecond)

	s, err = updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(runningState), string(s.State))
	assert.Equal(t, "Keystore updated", string(s.Reason))
}

func TestStart_ReloadFailure(t *testing.T) {
	updater, err := NewUpdater(Config{
		ReloadCredentials: true,
	}, ClientKoMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	go updater.Start()
	time.Sleep(timeToStartEs * 50)

	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(failedState), string(s.State))
}

func TestStart_KeystoreUpdateFailure(t *testing.T) {
	sourcePath, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(sourcePath) }()

	updater, err := NewUpdater(Config{
		SecretsSourceDir:  sourcePath,
		ReloadCredentials: false,
	}, ClientOkMock{}, KeystoreKoMock{})
	assert.NoError(t, err)

	go updater.Start()
	time.Sleep(timeToStartEs * 2)

	s, err := updater.Status()
	assert.NoError(t, err)
	assert.Equal(t, string(failedState), string(s.State))
}
