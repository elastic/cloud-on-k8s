// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package trial

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var cfg *rest.Config

func TestMain(m *testing.M) {
	apis.AddToScheme(scheme.Scheme) // here to avoid import cycle
	test.RunWithK8s(m, filepath.Join("..", "..", "..", "..", "config", "crds"))
}

// SetupTestReconcile returns a reconcile.Reconcile implementation that delegates to inner and
// writes the request to requests after Reconcile is finished.
func SetupTestReconcile(inner reconcile.Reconciler) (reconcile.Reconciler, chan reconcile.Request) {
	requests := make(chan reconcile.Request)
	fn := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
		requests <- req
		return inner.Reconcile(req)
	})
	return fn, requests
}

// StartTestManager adds recFn
func StartTestManager(mgr manager.Manager, t *testing.T) (chan struct{}, *sync.WaitGroup) {
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		assert.NoError(t, mgr.Start(stop))
		wg.Done()
	}()
	return stop, wg
}
