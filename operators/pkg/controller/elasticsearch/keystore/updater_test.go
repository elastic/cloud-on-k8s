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

	"github.com/elastic/k8s-operators/operators/pkg/utils/test"
	"github.com/stretchr/testify/assert"
)

const (
	timeToStartEs = 100 * time.Millisecond
)

type EsClientOkMock struct{}

func (c EsClientOkMock) ReloadSecureSettings() error { return nil }
func (c EsClientOkMock) WaitForEsReady()             { time.Sleep(timeToStartEs) }

type EsClientNeverReadyMock struct{}

func (c EsClientNeverReadyMock) ReloadSecureSettings() error { return nil }
func (c EsClientNeverReadyMock) WaitForEsReady()             { select {} }

type EsClientKoMock struct{}

func (c EsClientKoMock) ReloadSecureSettings() error { return errors.New("failed") }
func (c EsClientKoMock) WaitForEsReady()             { time.Sleep(timeToStartEs) }

type KeystoreOkMock struct{}

func (k KeystoreOkMock) Create() error                          { return nil }
func (k KeystoreOkMock) Delete() (bool, error)                  { return true, nil }
func (k KeystoreOkMock) ListSettings() (string, error)          { return "", nil }
func (k KeystoreOkMock) AddFileSetting(filename string) error   { return nil }
func (k KeystoreOkMock) AddStringSetting(filename string) error { return nil }
func (k KeystoreOkMock) AddSetting(filename string) error       { return nil }

type KeystoreKoMock struct{}

func (k KeystoreKoMock) Create() error                          { return errors.New("failed") }
func (k KeystoreKoMock) Delete() (bool, error)                  { return true, nil }
func (k KeystoreKoMock) ListSettings() (string, error)          { return "", nil }
func (k KeystoreKoMock) AddFileSetting(filename string) error   { return nil }
func (k KeystoreKoMock) AddStringSetting(filename string) error { return nil }
func (k KeystoreKoMock) AddSetting(filename string) error       { return nil }

func TestCoalescingRetry_Ok(t *testing.T) {
	updater, err := NewUpdater(Config{
		ReloadCredentials: true,
	}, EsClientOkMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	// Add an item in the queue
	assert.Equal(t, 0, updater.reloadQueue.Len())
	updater.reloadQueue.Add(attemptReload)
	assert.Equal(t, 1, updater.reloadQueue.Len())

	// Start coalescingRetry in background
	go updater.coalescingRetry()

	test.RetryUntilSuccess(t, func() error {
		if updater.reloadQueue.Len() != 0 {
			return errors.New("reload queue should be empty")
		}
		return nil
	})

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

	// Start coalescingRetry in background
	go updater.coalescingRetry()

	test.RetryUntilSuccess(t, func() error {
		if updater.reloadQueue.Len() != 0 {
			return errors.New("reload queue should be empty")
		}
		return nil
	})

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

	// Start watchForUpdate in background
	go updater.watchForUpdate()

	test.RetryUntilSuccess(t, func() error {
		if updater.reloadQueue.Len() != 1 {
			return errors.New("reload queue must be filled")
		}
		return nil
	})

	// Consume the queue manually
	item, _ := updater.reloadQueue.Get()
	updater.reloadQueue.Done(item)
	assert.Equal(t, 0, updater.reloadQueue.Len())

	// Write a new settings to be stored in the keystore
	_ = ioutil.WriteFile(filepath.Join(sourcePath, "setting1"), []byte("secret1"), 0644)

	test.RetryUntilSuccess(t, func() error {
		if updater.reloadQueue.Len() != 1 {
			return errors.New("reload queue must be filled")
		}
		return nil
	})

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
	}, EsClientNeverReadyMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	go updater.Start()

	test.RetryUntilSuccess(t, func() error {
		s, err := updater.Status()
		assert.NoError(t, err)

		if s.State != waitingState {
			return errors.New("state must be waiting")
		}
		return nil
	})
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

	test.RetryUntilSuccess(t, func() error {
		s, err := updater.Status()
		assert.NoError(t, err)

		if s.State != runningState {
			return errors.New("status must be running")
		}
		if s.Reason != keystoreUpdatedReason {
			return errors.New("keystore must be updated")
		}
		return nil
	})
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

	test.RetryUntilSuccess(t, func() error {
		s, err := updater.Status()
		assert.NoError(t, err)

		if s.State != runningState {
			return errors.New("status must be running")
		}
		if s.Reason != secureSettingsReloadedReason {
			return errors.New("secure settings must be reloaded")
		}
		return nil
	})

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

	test.RetryUntilSuccess(t, func() error {
		s, err := updater.Status()
		assert.NoError(t, err)

		if s.State != runningState {
			return errors.New("status must be running")
		}
		if s.Reason != keystoreUpdatedReason {
			return errors.New("keystore must be updated")
		}
		return nil
	})

	// Write a secure setting
	_ = ioutil.WriteFile(filepath.Join(sourcePath, "setting1"), []byte("secret1"), 0644)

	test.RetryUntilSuccess(t, func() error {
		s, err := updater.Status()
		assert.NoError(t, err)

		if s.State != runningState {
			return errors.New("status must be running")
		}
		if s.Reason != keystoreUpdatedReason {
			return errors.New("keystore must be updated")
		}
		return nil
	})
}

func TestStart_ReloadFailure(t *testing.T) {
	updater, err := NewUpdater(Config{
		ReloadCredentials: true,
	}, EsClientKoMock{}, KeystoreOkMock{})
	assert.NoError(t, err)

	go updater.Start()

	test.RetryUntilSuccess(t, func() error {
		s, err := updater.Status()
		assert.NoError(t, err)

		if s.State != failedState {
			return errors.New("status must be failed")
		}
		return nil
	})
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

	test.RetryUntilSuccess(t, func() error {
		s, err := updater.Status()
		assert.NoError(t, err)

		if s.State != failedState {
			return errors.New("status must be failed")
		}
		return nil
	})
}
