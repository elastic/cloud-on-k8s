// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package k8s

import (
	"errors"
	"io/ioutil"
	"os"
)

const (
	// InClusterNamespaceFile contains the name of the namespace in which the current
	// pod is deployed, when running in kubernetes
	InClusterNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

// CurrentPodName tries to guess the name of the pod this program is running in
// - first by looking at $POD_NAMESPACE
// - then by looking at InClusterNamespaceFile
func CurrentPodName() (string, error) {
	if fromEnv := os.Getenv("POD_NAMESPACE"); fromEnv != "" {
		return fromEnv, nil
	}
	fromFS, err := ioutil.ReadFile(InClusterNamespaceFile)
	return string(fromFS), err
}

// CurrentNamespace tries to guess the namespace this program is running in
// - first by looking at $POD_NAME
// - then by looking at /etc/hostname
func CurrentNamespace() (string, error) {
	if fromEnv := os.Getenv("POD_NAME"); fromEnv != "" {
		return fromEnv, nil
	}
	fromFSBytes, err := ioutil.ReadFile("/etc/hostname")
	if err != nil {
		return "", err
	}
	asStr := string(fromFSBytes)
	if asStr == "localhost" {
		// some k8s distributions may write the hostname as "localhost",
		// which does not make much sense in most contexts, return an error
		return "", errors.New("pod name advertised as localhost in /etc/hostname")
	}
	return asStr, err
}
