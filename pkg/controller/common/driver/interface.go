// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// Interface describes attributes typically found on reconciler or 'driver' implementations.
type Interface[T client.Object] interface {
	K8sClient() k8s.Client
	DynamicWatches() watches.DynamicWatches[T]
	Recorder() record.EventRecorder
}

// TestDriver is a struct implementing the common driver interface for testing purposes.
type TestDriver[T client.Object] struct {
	Client       k8s.Client
	Watches      watches.DynamicWatches[T]
	FakeRecorder *record.FakeRecorder
}

// K8sClient returns the kubernetes client from the APM Server reconciler.
func (t TestDriver[T]) K8sClient() k8s.Client {
	return t.Client
}

// DynamicWatches returns the set of dynamic watches from the APM Server reconciler.
func (t TestDriver[T]) DynamicWatches() watches.DynamicWatches[T] {
	return t.Watches
}

// Recorder returns the Kubernetes recorder that is responsible for recording and reporting
// events from the APM Server reconciler.
func (t TestDriver[T]) Recorder() record.EventRecorder {
	return t.FakeRecorder
}

var _ Interface[client.Object] = &TestDriver[client.Object]{}
