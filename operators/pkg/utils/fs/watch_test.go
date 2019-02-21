// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package fs

import (
	"errors"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func Test_WatchPath(t *testing.T) {
	// setup a tmp file to watch
	tmpFile, err := ioutil.TempFile("", "tmpfile")
	require.NoError(t, err)
	path := tmpFile.Name()
	defer os.Remove(path)

	logger := logf.Log.WithName("test")
	events := make(chan string)

	// on each event, read the file
	// according to its content, return in different ways
	f := func() (bool, error) {
		content, err := ioutil.ReadFile(path)
		require.NoError(t, err)
		switch string(content) {
		case "stop":
			events <- "stop"
			return true, nil
		case "error":
			events <- "error"
			return false, errors.New("error")
		default:
			events <- "continue"
			return false, nil
		}
	}

	done := make(chan error)
	go func() {
		done <- WatchPath(path, f, logger)
	}()

	// should trigger an event before actually watching
	evt := <-events
	require.Equal(t, "continue", evt)

	// trigger a change that should continue
	require.NoError(t, ioutil.WriteFile(path, []byte("continue"), 644))
	require.Equal(t, "continue", <-events)
	// again
	require.NoError(t, ioutil.WriteFile(path, []byte("continue"), 644))
	require.Equal(t, "continue", <-events)

	// trigger a change that should stop
	require.NoError(t, ioutil.WriteFile(path, []byte("stop"), 644))
	require.Equal(t, "stop", <-events)
	// WatchPath should return
	require.NoError(t, <-done)

	// run it again
	require.NoError(t, ioutil.WriteFile(path, []byte("continue"), 644))
	done = make(chan error)
	go func() {
		done <- WatchPath(path, f, logger)
	}()

	// should trigger an event before actually watching
	require.Equal(t, "continue", <-events)

	// trigger a change that should return an error
	require.NoError(t, ioutil.WriteFile(path, []byte("error"), 644))
	require.Equal(t, "error", <-events)

	// WatchPath should return with an error
	require.Error(t, <-done, "error")
}
