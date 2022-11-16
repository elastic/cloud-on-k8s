// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package tracing

import (
	"go.elastic.co/apm/v2"

	"github.com/elastic/cloud-on-k8s/v2/pkg/about"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

var (
	log = ulog.Log.WithName("tracing")
)

// NewTracer returns a new APM tracer with the logger in log configured.
func NewTracer(serviceName string) *apm.Tracer {
	build := about.GetBuildInfo()
	tracer, err := apm.NewTracer(serviceName, build.Version+"-"+build.Hash)
	if err != nil {
		// don't fail the application because tracing fails
		log.Error(err, "failed to created tracer for "+serviceName)
		return nil
	}
	tracer.SetLogger(NewLogAdapter(log))
	return tracer
}
