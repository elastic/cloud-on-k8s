// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package net

import (
	"net"
)

// GetRandomPort returns a random port chosen by the OS by binding to :0 and checking what port was bound.
func GetRandomPort() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	if err := listener.Close(); err != nil {
		return "", err
	}

	_, localPort, err := net.SplitHostPort(listener.Addr().String())
	return localPort, err
}
