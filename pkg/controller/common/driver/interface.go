// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	toolsevents "k8s.io/client-go/tools/events"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// Interface describes attributes typically found on reconciler or 'driver' implementations.
type Interface interface {
	K8sClient() k8s.Client
	DynamicWatches() watches.DynamicWatches
	Recorder() toolsevents.EventRecorder
}

// TestDriver is a struct implementing the common driver interface for testing purposes.
type TestDriver struct {
	Client       k8s.Client
	Watches      watches.DynamicWatches
	FakeRecorder *toolsevents.FakeRecorder
}

// K8sClient returns the kubernetes client from the APM Server reconciler.
func (t TestDriver) K8sClient() k8s.Client {
	return t.Client
}

// DynamicWatches returns the set of dynamic watches from the APM Server reconciler.
func (t TestDriver) DynamicWatches() watches.DynamicWatches {
	return t.Watches
}

// Recorder returns the Kubernetes recorder that is responsible for recording and reporting
// events from the APM Server reconciler.
func (t TestDriver) Recorder() toolsevents.EventRecorder {
	return t.FakeRecorder
}

var _ Interface = (*TestDriver)(nil)
