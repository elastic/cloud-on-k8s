// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"time"

	"go.elastic.co/apm/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

var _ manager.LeaderElectionRunnable = ResourceReporter{}

// ResourceReporterFrequency defines the reporting frequency of the resource reporter
const ResourceReporterFrequency = 2 * time.Minute

// ResourceReporter aggregates resources of all Elastic components managed by the operator
// and reports them in a config map in the form of licensing information. It satisfies
// manager.LeaderElectionRunnable.
type ResourceReporter struct {
	aggregator        aggregator
	licensingResolver LicensingResolver
	tracer            *apm.Tracer
	refreshPeriod     time.Duration
}

// NewResourceReporter returns a new ResourceReporter. A non-positive refreshPeriod defaults to ResourceReporterFrequency.
func NewResourceReporter(c client.Client, operatorNs string, tracer *apm.Tracer, refreshPeriod time.Duration) ResourceReporter {
	if refreshPeriod <= 0 {
		refreshPeriod = ResourceReporterFrequency
	}
	return ResourceReporter{
		aggregator: aggregator{
			client: c,
		},
		licensingResolver: LicensingResolver{
			client:     c,
			operatorNs: operatorNs,
		},
		tracer:        tracer,
		refreshPeriod: refreshPeriod,
	}
}

func (r ResourceReporter) NeedLeaderElection() bool { return true }

// Start starts to report the licensing information repeatedly at regular intervals
func (r ResourceReporter) Start(ctx context.Context) error {
	ctx = ulog.InitInContext(ctx, "resource-reporter")
	log := ulog.FromContext(ctx)
	// report once as soon as possible to not wait the first tick
	if err := r.Report(ctx); err != nil {
		log.Error(err, "Failed to report licensing information")
	}

	ticker := time.NewTicker(r.refreshPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := r.Report(ctx); err != nil {
				log.Error(err, "Failed to report licensing information")
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// Report licensing information by publishing metrics and updating the config map.
func (r ResourceReporter) Report(ctx context.Context) error {
	ctx = tracing.NewContextTransaction(ctx, r.tracer, tracing.PeriodicTxType, "resource-reporter", nil)
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
	usage, err := r.aggregator.aggregateMemory(ctx)
	if err != nil {
		return LicensingInfo{}, err
	}

	return r.licensingResolver.ToInfo(ctx, usage)
}
