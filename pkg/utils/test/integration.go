// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	controllerscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"

	"github.com/stretchr/testify/require"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
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

	CRDsRelativePath = "../../../config/crds/all-crds.yaml"
)

var Config *rest.Config

// RunWithK8s starts a local Kubernetes server and runs tests in m.
func RunWithK8s(m *testing.M) {
	// add CRDs scheme to the client
	_ = controllerscheme.SetupScheme()

	t := &envtest.Environment{
		CRDs:                     parseCRDs(),
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

// parseCRDs parses the content of CRDsRelativePath into a list of CustomResourceDefinitions.
func parseCRDs() []*v1beta1.CustomResourceDefinition {
	// read CRDsRelativePath relatively to the current file path
	_, currentFilePath, _, ok := runtime.Caller(1)
	if !ok {
		panic("Cannot retrieve path to the current file")
	}
	crdsFile := filepath.Join(filepath.Dir(currentFilePath), CRDsRelativePath)

	// parse the yaml file into multiple CRDs
	yamlFile, err := os.Open(crdsFile)
	if err != nil {
		panic("Cannot read file " + crdsFile + ": " + err.Error())
	}
	decoder := yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile))
	var crds []*v1beta1.CustomResourceDefinition
	for {
		var crd v1beta1.CustomResourceDefinition
		err := decoder.Decode(&crd)
		if err == io.EOF {
			break
		}
		if err != nil {
			panic("Cannot parse CRD" + err.Error())
		}
		crd.Spec.Validation.OpenAPIV3Schema.Type = ""
		crds = append(crds, &crd)
	}
	if len(crds) == 0 {
		panic("No CRD parsed in " + crdsFile)
	}
	fmt.Println("parsed ", len(crds))
	return crds
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
