// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package processmanager

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/utils/net"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func Test_Simple_Script(t *testing.T) {
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

func Test_Zombies_Script(t *testing.T) {
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

func Test_ZombiesAndTrap_Script(t *testing.T) {
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

func Test_Recursive_Script(t *testing.T) {
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

func Test_Invalid_Command(t *testing.T) {
	setupEnv(t, "invalid_command")
	procMgr, err := NewProcessManager()
	assert.NoError(t, err)
	err = procMgr.Start()
	assert.Error(t, err)
}

func setupEnv(t *testing.T, cmd string) string {
	port, err := net.GetRandomPort()
	assert.NoError(t, err)
	err = os.Setenv(EnvHTTPPort, port)
	assert.NoError(t, err)
	err = os.Setenv(EnvProcName, "test")
	assert.NoError(t, err)
	err = os.Setenv(EnvProcCmd, cmd)
	assert.NoError(t, err)
	err = os.Setenv(EnvKeystoreUpdater, "false")
	assert.NoError(t, err)
	err = BindFlagsToEnv(&cobra.Command{})
	assert.NoError(t, err)
	return port
}

func runTest(t *testing.T, cmd string, do func(client *Client)) {
	port := setupEnv(t, cmd)
	procMgr, err := NewProcessManager()
	assert.NoError(t, err)
	err = procMgr.Start()
	assert.NoError(t, err)

	client := NewClient("http://localhost:"+port, nil)

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
