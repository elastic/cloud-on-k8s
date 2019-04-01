// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package processmanager

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/utils/net"
	"github.com/stretchr/testify/assert"
)

func TestSimpleScript(t *testing.T) {
	runTest(t, "fixtures/script", func(client *Client) {
		assertState(t, client, started)

		// stopping
		status, err := client.Stop(context.Background())
		assert.NoError(t, err)
		assertEqual(t, stopping, status.State)
		time.Sleep(10 * time.Millisecond)

		assertState(t, client, stopped)

		// starting
		status, err = client.Start(context.Background())
		assert.NoError(t, err)
		assertEqual(t, started, status.State)
		time.Sleep(10 * time.Millisecond)

		assertState(t, client, started)

		// stopping
		status, err = client.Stop(context.Background())
		assert.NoError(t, err)
		assertEqual(t, stopping, status.State)
		time.Sleep(10 * time.Millisecond)

		assertState(t, client, stopped)
	})
}

func TestZombiesScript(t *testing.T) {
	runTest(t, "fixtures/script zombies", func(client *Client) {
		assertState(t, client, started)

		// stopping
		status, err := client.Stop(context.Background())
		assert.NoError(t, err)
		assertEqual(t, stopping, status.State)
		time.Sleep(10 * time.Millisecond)

		assertState(t, client, stopped)

		// starting
		status, err = client.Start(context.Background())
		assert.NoError(t, err)
		assertEqual(t, started, status.State)
		time.Sleep(10 * time.Millisecond)

		assertState(t, client, started)

		// stopping
		status, err = client.Stop(context.Background())
		assert.NoError(t, err)
		assertEqual(t, stopping, status.State)
		time.Sleep(10 * time.Millisecond)

		assertState(t, client, stopped)
	})
}

func TestZombiesAndTrapScript(t *testing.T) {
	runTest(t, "fixtures/script zombies trap", func(client *Client) {
		assertState(t, client, started)

		// stopping
		status, err := client.Stop(context.Background())
		assert.NoError(t, err)
		assertEqual(t, stopping, status.State)
		time.Sleep(10 * time.Millisecond)

		// starting should fail because the stop is still in progress
		status, err = client.Start(context.Background())
		assert.Error(t, err)
		assertEqual(t, stopping, status.State)

		assertState(t, client, stopping)

		// stopping
		status, err = client.Stop(context.Background())
		assert.NoError(t, err)
		assertEqual(t, stopping, status.State)
		time.Sleep(10 * time.Millisecond)

		assertState(t, client, stopping)

		// killing
		status, err = client.Kill(context.Background())
		assert.NoError(t, err)
		assertEqual(t, killing, status.State)
		time.Sleep(10 * time.Millisecond)

		assertState(t, client, killed)
	})
}

func TestRecursiveScript(t *testing.T) {
	runTest(t, "fixtures/script zombies trap recursive", func(client *Client) {
		assertState(t, client, started)

		// stopping
		status, err := client.Stop(context.Background())
		assert.NoError(t, err)
		assertEqual(t, stopping, status.State)
		time.Sleep(10 * time.Millisecond)

		// starting should fail because the stop is still in progress
		status, err = client.Start(context.Background())
		assert.Error(t, err)
		assertEqual(t, stopping, status.State)

		assertState(t, client, stopping)

		// stopping
		status, err = client.Stop(context.Background())
		assert.NoError(t, err)
		assertEqual(t, stopping, status.State)
		time.Sleep(10 * time.Millisecond)

		assertState(t, client, stopping)

		// killing
		status, err = client.Kill(context.Background())
		assert.NoError(t, err)
		assertEqual(t, killing, status.State)
		time.Sleep(10 * time.Millisecond)

		assertState(t, client, killed)
	})
}

func TestInvalidCommand(t *testing.T) {
	cfg := newConfig(t, "invalid_command")
	procMgr, err := NewProcessManager(cfg)
	assert.NoError(t, err)
	err = procMgr.Start()
	assert.Error(t, err)
}

func newConfig(t *testing.T, cmd string) *Config {
	port, err := net.GetRandomPort()
	assert.NoError(t, err)

	HTTPPort, err := strconv.Atoi(port)
	assert.NoError(t, err)

	return &Config{
		ProcessName:           "test",
		ProcessCmd:            cmd,
		HTTPPort:              HTTPPort,
		EnableKeystoreUpdater: false,
	}
}

func runTest(t *testing.T, cmd string, do func(client *Client)) {
	cfg := newConfig(t, cmd)
	procMgr, err := NewProcessManager(cfg)
	assert.NoError(t, err)
	err = procMgr.Start()
	assert.NoError(t, err)

	client := NewClient(fmt.Sprintf("http://localhost:%d", cfg.HTTPPort), nil)

	time.Sleep(3 * time.Second)
	do(client)

	err = procMgr.Stop(os.Kill)
	assert.NoError(t, err)
}

func assertState(t *testing.T, client *Client, expectedState ProcessState) {
	status, err := client.Status(context.Background())
	assert.NoError(t, err)
	assertEqual(t, expectedState, status.State)
}

func assertEqual(t *testing.T, expected ProcessState, actual ProcessState) {
	assert.Equal(t, expected.String(), actual.String())
}
