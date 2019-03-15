// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.
//
// +build integration

package v1alpha1

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var cfg *rest.Config
var c client.Client

func TestMain(m *testing.M) {
	t := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "..", "config", "crds")},
	}

	err := SchemeBuilder.AddToScheme(scheme.Scheme)
	if err != nil {
		log.Fatal(err)
	}

	if cfg, err = t.Start(); err != nil {
		log.Fatal(err)
	}

	if c, err = client.New(cfg, client.Options{Scheme: scheme.Scheme}); err != nil {
		log.Fatal(err)
	}

	code := m.Run()
	t.Stop()
	os.Exit(code)
}
