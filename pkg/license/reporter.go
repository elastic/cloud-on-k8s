// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"time"

	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceReporterFrequency defines the reporting frequency of the resource reporter
const ResourceReporterFrequency = 2 * time.Minute

var log = ulog.Log.WithName("resource")

// ResourceReporter aggregates resources of all Elastic components managed by the operator
// and reports them in a config map in the form of licensing information
type ResourceReporter struct {
	aggregator        Aggregator
	licensingResolver LicensingResolver
}

// NewResourceReporter returns a new ResourceReporter
func NewResourceReporter(c client.Client, operatorNs string) ResourceReporter {
	return ResourceReporter{
		aggregator: Aggregator{
			client: c,
		},
		licensingResolver: LicensingResolver{
			client:     c,
			operatorNs: operatorNs,
		},
	}
}

// Start starts to report the licensing information repeatedly at regular intervals
func (r ResourceReporter) Start(refreshPeriod time.Duration) {
	// report once as soon as possible to not wait the first tick
	err := r.Report()
	if err != nil {
		log.Error(err, "Failed to report licensing information")
	}

	ticker := time.NewTicker(refreshPeriod)
	for range ticker.C {
		err := r.Report()
		if err != nil {
			log.Error(err, "Failed to report licensing information")
		}
	}
}

// Report licensing information by publishing metrics and updating the config map.
func (r ResourceReporter) Report() error {
	licensingInfo, err := r.Get()
	if err != nil {
		return err
	}

	licensingInfo.ReportAsMetrics()
	return r.licensingResolver.Save(licensingInfo)
}

// Get aggregates managed resources and returns the licensing information
func (r ResourceReporter) Get() (LicensingInfo, error) {
	totalMemory, err := r.aggregator.AggregateMemory()
	if err != nil {
		return LicensingInfo{}, err
	}

	return r.licensingResolver.ToInfo(totalMemory)
}
