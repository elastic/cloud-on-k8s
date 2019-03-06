// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package main

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func waitForServer(t *testing.T, port int) {
	// wait for server to be started
	totalTimeout := time.After(30 * time.Second)
	retryEvery := time.Tick(100 * time.Millisecond)
	reqTimeout := 10 * time.Second
	for {
		select {
		case <-totalTimeout:
			t.Fatal("server not reachable after 30sec.")
		case <-retryEvery:
			// check if TCP port listens to connections
			_, err := net.DialTimeout("tcp", net.JoinHostPort("", fmt.Sprintf("%d", port)), reqTimeout)
			if err == nil {
				return
			}
		}
	}
}

func Test_serveCSR(t *testing.T) {
	// try to find an open tcp port for using in the test,
	// by binding to :0 and getting the assigned port
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	require.NoError(t, err)
	l, err := net.ListenTCP("tcp", addr)
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	// start the server in a goroutine
	privateKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	require.NoError(t, err)
	csr, err := createCSR(privateKey)
	require.NoError(t, err)
	stopChan := make(chan struct{})
	isStopped := make(chan struct{})
	go func() {
		err := serveCSR(port, csr, stopChan)
		require.NoError(t, err)
		close(isStopped)
	}()

	waitForServer(t, port)

	// request the csr
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/csr", port))
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, csr, data)

	// request again (idempotent request)
	resp, err = http.Get(fmt.Sprintf("http://localhost:%d/csr", port))
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, csr, data)

	// stopping the server should behave as expected
	// (error is checked in the goroutine above)
	close(stopChan)
	<-isStopped
}
