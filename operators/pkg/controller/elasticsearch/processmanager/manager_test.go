// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package processmanager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/utils/net"
	"github.com/stretchr/testify/assert"
)

const testCmd = "fixtures/simulate-child-processes.sh"

func TestScriptDieAlone(t *testing.T) {
	runTest(t, testCmd, ExitStatus{"completed", 0, nil}, func(client *Client) {
		time.Sleep(4 * time.Second)
		// should die alone after a few seconds
	})
}

func TestStopScriptForever(t *testing.T) {
	runTest(t, testCmd+" forever", ExitStatus{"exited", -1, nil}, func(client *Client) {
		// stopping
		status, err := client.Stop(context.Background())
		assert.NoError(t, err)
		assertEqual(t, stopping, status.State)
	})
}

func TestKillScriptForever(t *testing.T) {
	runTest(t, testCmd+" forever", ExitStatus{"killed", -1, nil}, func(client *Client) {
		// killing
		status, err := client.Kill(context.Background())
		assert.NoError(t, err)
		assertEqual(t, killing, status.State)
	})
}

func TestStopScriptForeverWithTrap(t *testing.T) {
	runTest(t, testCmd+" forever enableTrap", ExitStatus{"exited", 143, nil}, func(client *Client) {
		assertState(t, client, started)

		// stopping
		status, err := client.Stop(context.Background())
		assert.NoError(t, err)
		assertEqual(t, stopping, status.State)

		time.Sleep(4 * time.Second)
		// should die alone after a few seconds
	})
}

func TestKillScriptForeverWithTrap(t *testing.T) {
	runTest(t, testCmd+" forever enableTrap", ExitStatus{"killed", -1, nil}, func(client *Client) {
		assertState(t, client, started)

		// killing
		status, err := client.Kill(context.Background())
		assert.NoError(t, err)
		assertEqual(t, killing, status.State)
	})
}

func TestStopAndKillScriptForeverWithTrapForever(t *testing.T) {
	runTest(t, testCmd+" forever enableTrap forever", ExitStatus{"killed", -1, nil}, func(client *Client) {
		// stopping
		status, err := client.Stop(context.Background())
		assert.NoError(t, err)
		assertEqual(t, stopping, status.State)

		// still stopping
		time.Sleep(3 * time.Second)
		assertState(t, client, stopping)

		// killing
		status, err = client.Kill(context.Background())
		assert.NoError(t, err)
		assertEqual(t, killing, status.State)
	})
}

func TestInvalidCommand(t *testing.T) {
	resetProcessStateFile(t)
	cfg := newConfig(t, "invalid_command")
	procMgr, err := NewProcessManager(cfg)
	assert.NoError(t, err)

	done := make(chan ExitStatus)
	err = procMgr.Start(done)
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

func runTest(t *testing.T, cmd string, expected ExitStatus, do func(client *Client)) {
	resetProcessStateFile(t)

	cfg := newConfig(t, cmd)
	procMgr, err := NewProcessManager(cfg)
	assert.NoError(t, err)

	done := make(chan ExitStatus)
	err = procMgr.Start(done)
	assert.NoError(t, err)

	time.Sleep(1 * time.Second)
	client := NewClient(fmt.Sprintf("http://localhost:%d", cfg.HTTPPort), nil)
	assertState(t, client, started)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		for {
			select {
			case <-ctx.Done():
				err := procMgr.Forward(killHardSignal)
				assert.NoError(t, err)
				assert.Error(t, errors.New("process manager not stopped"))
				wg.Done()
				return
			case current := <-done:
				cancel()
				close(done)
				assert.Equal(t, expected.exitCode, current.exitCode)
				assert.Equal(t, expected.processStatus, current.processStatus)

				wg.Done()
				return
			}
		}
	}()

	go do(client)

	wg.Wait()

	_, err = syscall.Getpgid(procMgr.process.pid)
	assert.Error(t, err)
	assert.Error(t, err)

	procMgr.server.Exit()

	_, err = client.Status(context.Background())
	assert.Error(t, err)
}

func resetProcessStateFile(t *testing.T) {
	_ = os.Remove(processStateFile)
}

func assertState(t *testing.T, client *Client, expectedState ProcessState) {
	status, err := client.Status(context.Background())
	assert.NoError(t, err)
	assertEqual(t, expectedState, status.State)
}

func assertEqual(t *testing.T, expected ProcessState, actual ProcessState) {
	assert.Equal(t, expected.String(), actual.String())
}
