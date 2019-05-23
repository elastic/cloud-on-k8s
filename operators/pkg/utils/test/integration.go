// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
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

// StartManager sets up a manager and controller to perform reconciliations in background.
// It must be stopped by calling the returned function.
func StartManager(t *testing.T, addToMgrFunc func(manager.Manager, operator.Parameters) error, parameters operator.Parameters) (k8s.Client, func()) {
	mgr, err := manager.New(Config, manager.Options{})
	require.NoError(t, err)

	err = addToMgrFunc(mgr, parameters)
	require.NoError(t, err)

	stopChan := make(chan struct{})
	stopped := make(chan error)
	// run the manager in background, until stopped
	go func() {
		stopped <- mgr.Start(stopChan)
	}()

	client := k8s.WrapClient(mgr.GetClient())
	stopFunc := func() {
		// stop the manager and wait until stopped
		close(stopChan)
		require.NoError(t, <-stopped)
	}

	return client, stopFunc
}
