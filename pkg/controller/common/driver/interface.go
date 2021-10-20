// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"k8s.io/client-go/tools/record"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// Interface describes attributes typically found on reconciler or 'driver' implementations.
type Interface interface {
	K8sClient() k8s.Client
	DynamicWatches() watches.DynamicWatches
	Recorder() record.EventRecorder
}

// TestDriver is a struct implementing the common driver interface for testing purposes.
type TestDriver struct {
	Client       k8s.Client
	Watches      watches.DynamicWatches
	FakeRecorder *record.FakeRecorder
}

func (t TestDriver) K8sClient() k8s.Client {
	return t.Client
}

func (t TestDriver) DynamicWatches() watches.DynamicWatches {
	return t.Watches
}

func (t TestDriver) Recorder() record.EventRecorder {
	return t.FakeRecorder
}

var _ Interface = &TestDriver{}
