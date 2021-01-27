// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	controllerscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// ControlPlaneStartTimeout is the time to wait for control plane startup
	// in kubebuilder integration tests.
	// It is set at a relatively high value due to low resources in continuous integration.
	ControlPlaneStartTimeout = 1 * time.Minute
	BootstrapTestEnvRetries  = 1

	CRDsRelativePath = "../../../config/crds"
)

var Config *rest.Config

// RunWithK8s starts a local Kubernetes server and runs tests in m.
func RunWithK8s(m *testing.M) {
	// add CRDs scheme to the client
	controllerscheme.SetupScheme()

	t := &envtest.Environment{
		CRDDirectoryPaths:        []string{CRDsRelativePath},
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
	mgr, err := manager.New(Config, manager.Options{
		MetricsBindAddress: "0", // disable
	})
	require.NoError(t, err)

	err = addToMgrFunc(mgr, parameters)
	require.NoError(t, err)

	ctx, cancelFunc := context.WithCancel(context.Background())
	errChan := make(chan error)

	// run the manager in background, until stopped
	go func() {
		errChan <- mgr.Start(ctx)
	}()

	mgr.GetCache().WaitForCacheSync(ctx) // wait until k8s client cache is initialized

	client := mgr.GetClient()
	stopFunc := func() {
		cancelFunc()
		require.NoError(t, <-errChan)
	}

	return client, stopFunc
}
