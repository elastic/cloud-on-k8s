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
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/test"
	"github.com/stretchr/testify/assert"
)

var (
	imageName     string
	containerName string
	cmd           = "/usr/local/bin/docker-entrypoint.sh"
)

func TestMain(m *testing.M) {
	setup()
	retCode := m.Run()
	teardown()

	os.Exit(retCode)
}

func setup() {
	randName := fmt.Sprintf("%s-%d", "pm-test", time.Now().UnixNano())
	imageName = randName
	containerName = randName

	// Build a Docker image based on Elasticsearch with the process manager
	err := exec.Command("docker", "build",
		"-f", "tests/Dockerfile",
		"-t", imageName, "../../../..").Run()
	if err != nil {
		log.Error(err, "Failed to build docker image")
		os.Exit(1)
	}
}

func teardown() {
	rmContainer()
}

func bash(format string, a ...interface{}) *exec.Cmd {
	return exec.Command("bash", "-c", fmt.Sprintf(format, a...))
}

func rmContainer() {
	_ = bash("docker rm -f %s", containerName).Run()
}

func startContainer(t *testing.T, cmd string) Client {
	// Always clean up the container before starting another one
	rmContainer()

	port, err := net.GetRandomPort()
	assert.NoError(t, err)
	err = exec.Command("docker", "run", "-d",
		"--name", containerName,
		"-p", port+":8080",
		"-e", "PM_PROC_NAME=es", "-e", "PM_PROC_CMD="+cmd,
		"-e", "PM_KEYSTORE_UPDATER=false",
		imageName).Start()
	assert.NoError(t, err)
	waitForContainerReady(t)

	client := NewClient(fmt.Sprintf("http://%s:%s", "localhost", port), nil, nil)
	assertProcessStatus(t, client, Started)

	return client
}

func restartContainer(t *testing.T) {
	err := bash("docker start %s", containerName).Run()
	assert.NoError(t, err)
	waitForContainerReady(t)
}

func waitForContainerReady(t *testing.T) {
	test.RetryUntilSuccess(t, func() error {
		return bash("docker exec %s sh -c exit", containerName).Run()
	})
}

func getProcessPID(t *testing.T) string {
	out, err := bash("docker exec %s ps -eo pid,cmd | grep org.elasticsearch.bootstrap.Elasticsearch | awk '{print $1}'", containerName).Output()
	assert.NoError(t, err)
	return string(out)
}

func assertContainerExited(t *testing.T) {
	test.RetryUntilSuccess(t, func() error {
		out, err := bash(`docker ps --all --filter=name=%s --format="{{.Status}}"`, containerName).Output()
		if err != nil {
			return err
		}
		if !strings.Contains(string(out), "Exited") {
			return errors.New("container should be exited")
		}
		return nil
	})
}

func assertProcessStatus(t *testing.T, client Client, expectedState ProcessState) {
	test.RetryUntilSuccess(t, func() error {
		status, err := client.Status(context.Background())
		if err != nil {
			return err
		}
		if expectedState.String() != status.State.String() {
			return errors.New("container should be exited")
		}
		if status.State == Started {
			if getProcessPID(t) == "" {
				return errors.New("PID should not be empty")
			}
		} else {
			if getProcessPID(t) != "" {
				return errors.New("PID should be empty")
			}
		}
		return nil
	})
}

// -- Tests

func Test_ApiStop(t *testing.T) {
	client := startContainer(t, cmd)

	_, err := client.Stop(context.Background())
	assert.NoError(t, err)
	assertContainerExited(t)

	restartContainer(t)
	assertProcessStatus(t, client, Stopped)
}

func Test_ApiKill(t *testing.T) {
	client := startContainer(t, cmd)

	_, err := client.Kill(context.Background())
	assert.NoError(t, err)
	assertContainerExited(t)

	restartContainer(t)
	assertProcessStatus(t, client, Killed)
}

func Test_Kill15Process(t *testing.T) {
	client := startContainer(t, cmd)

	err := bash("docker exec %s kill -15 %s", containerName, getProcessPID(t)).Run()
	assert.NoError(t, err)
	assertContainerExited(t)

	restartContainer(t)
	assertProcessStatus(t, client, Started)
}

func Test_Kill9Process(t *testing.T) {
	client := startContainer(t, cmd)

	err := bash("docker exec %s kill -9 %s", containerName, getProcessPID(t)).Run()
	assert.NoError(t, err)
	assertContainerExited(t)

	restartContainer(t)
	assertProcessStatus(t, client, Started)
}

func Test_Kill15ProcessManager(t *testing.T) {
	client := startContainer(t, cmd)

	err := bash("docker exec %s kill -15 1", containerName).Run()
	assert.NoError(t, err)
	assertContainerExited(t)

	restartContainer(t)
	assertProcessStatus(t, client, Started)
}

func Test_DockerStop(t *testing.T) {
	client := startContainer(t, cmd)

	err := bash("docker stop %s", containerName).Run()
	assert.NoError(t, err)
	assertContainerExited(t)

	restartContainer(t)
	assertProcessStatus(t, client, Started)
}

func Test_DockerKill(t *testing.T) {
	client := startContainer(t, cmd)

	err := bash("docker kill %s", containerName).Run()
	assert.NoError(t, err)
	assertContainerExited(t)

	restartContainer(t)
	assertProcessStatus(t, client, Started)
}
