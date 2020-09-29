// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package tracing

import (
	"sync"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"go.elastic.co/apm"
)

const serviceName = "elastic-operator"

var (
	tracer     *apm.Tracer
	initTracer sync.Once
)

// InitTracer initializes the global tracer for the application.
func InitTracer() error {
	var err error
	initTracer.Do(func() {
		build := about.GetBuildInfo()
		t, err := apm.NewTracer(serviceName, build.VersionString())
		if err != nil {
			errors.Wrap(err, "failed to initialize tracer")
		}
		tracer = t
	})
	return err
}

// Tracer returns the currently configured tracer.
func Tracer() *apm.Tracer {
	return tracer
}

// SetLogger sets the logger for the tracer.
func SetLogger(log logr.Logger) {
	if tracer != nil {
		tracer.SetLogger(NewLogAdapter(log))
	}
}
