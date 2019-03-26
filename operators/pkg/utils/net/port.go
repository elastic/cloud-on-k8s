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
