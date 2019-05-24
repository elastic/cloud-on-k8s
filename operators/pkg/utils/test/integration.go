// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	// ControlPlaneStartTimeout is the time to wait for control plane startup
	// in kubebuilder integration tests.
	// It is set at a relatively high value due to low resources in continuous integration.
	ControlPlaneStartTimeout = 2 * time.Minute
	BootstrapTestEnvRetries  = 3
)

var Config *rest.Config
var log = logf.Log.WithName("integration-test")

// RunWithK8s starts a local Kubernetes server and runs tests in m.
func RunWithK8s(m *testing.M, crdPath string) {
	if err := apis.AddToScheme(scheme.Scheme); err != nil {
		fmt.Println("fail to add scheme")
		panic(err)
	}

	logf.SetLogger(logf.ZapLogger(true))
	t := &envtest.Environment{
		CRDDirectoryPaths:        []string{crdPath},
		ControlPlaneStartTimeout: ControlPlaneStartTimeout,
	}

	// attempt to start the k8s test environment
	// this happens to fail for various reasons, let's retry up to BootstrapTestEnvRetries times
	retry := 1
	for {
		if retry > BootstrapTestEnvRetries {
			fmt.Printf("failed to start test environment after %d attempts, exiting.\n", BootstrapTestEnvRetries)
			os.Exit(1)
		}
		var err error
		Config, err = t.Start()
		if err == nil {
			break // test environment successfully started
		}
		fmt.Printf("failed to start test environment (attempt %d/%d): %s\n", retry, BootstrapTestEnvRetries, err.Error())
		retry++
		continue
	}

	code := m.Run()
	if err := t.Stop(); err != nil {
		fmt.Println("failed to stop test environment:", err.Error())
	}
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
