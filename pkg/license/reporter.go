// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"time"

	"go.elastic.co/apm"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
)

// ResourceReporterFrequency defines the reporting frequency of the resource reporter
const ResourceReporterFrequency = 2 * time.Minute

var log = ulog.Log.WithName("resource")

// ResourceReporter aggregates resources of all Elastic components managed by the operator
// and reports them in a config map in the form of licensing information
type ResourceReporter struct {
	aggregator        Aggregator
	licensingResolver LicensingResolver
	tracer            *apm.Tracer
}

// NewResourceReporter returns a new ResourceReporter
func NewResourceReporter(c client.Client, operatorNs string, tracer *apm.Tracer) ResourceReporter {
	return ResourceReporter{
		aggregator: Aggregator{
			client: c,
		},
		licensingResolver: LicensingResolver{
			client:     c,
			operatorNs: operatorNs,
		},
		tracer: tracer,
	}
}

// Start starts to report the licensing information repeatedly at regular intervals
func (r ResourceReporter) Start(ctx context.Context, refreshPeriod time.Duration) {
	// report once as soon as possible to not wait the first tick
	err := r.Report(ctx)
	if err != nil {
		log.Error(err, "Failed to report licensing information")
	}

	ticker := time.NewTicker(refreshPeriod)
	for range ticker.C {
		err := r.Report(ctx)
		if err != nil {
			log.Error(err, "Failed to report licensing information")
		}
	}
}

// Report licensing information by publishing metrics and updating the config map.
func (r ResourceReporter) Report(ctx context.Context) error {
	ctx = tracing.NewContextTransaction(ctx, r.tracer, "resource-reporter", "license_info", nil)
	defer tracing.EndContextTransaction(ctx)

	licensingInfo, err := r.Get(ctx)
	if err != nil {
		return err
	}

	licensingInfo.ReportAsMetrics()
	return r.licensingResolver.Save(ctx, licensingInfo)
}

// Get aggregates managed resources and returns the licensing information
func (r ResourceReporter) Get(ctx context.Context) (LicensingInfo, error) {
	span, _ := apm.StartSpan(ctx, "get_license_info", tracing.SpanTypeApp)
	defer span.End()
	totalMemory, err := r.aggregator.AggregateMemory()
	if err != nil {
		return LicensingInfo{}, err
	}

	return r.licensingResolver.ToInfo(totalMemory)
}
