// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"os"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	// ControlPlaneStartTimeout is the time to wait for control plane startup
	// in kubebuilder integration tests.
	// It is set at a relatively high value due to low resources in continuous integration.
	ControlPlaneStartTimeout = 2 * time.Minute
)

var Config *rest.Config
var log = logf.Log.WithName("integration-test")

// RunWithK8s starts a local Kubernetes server and runs tests in m.
func RunWithK8s(m *testing.M, crdPath string) {
	logf.SetLogger(logf.ZapLogger(true))
	t := &envtest.Environment{
		CRDDirectoryPaths:        []string{crdPath},
		ControlPlaneStartTimeout: ControlPlaneStartTimeout,
	}

	var err error
	if Config, err = t.Start(); err != nil {
		log.Error(err, "failed to start")
	}

	code := m.Run()
	t.Stop()
	os.Exit(code)
}
