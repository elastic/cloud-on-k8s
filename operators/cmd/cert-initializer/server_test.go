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

	"github.com/stretchr/testify/require"
)

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
