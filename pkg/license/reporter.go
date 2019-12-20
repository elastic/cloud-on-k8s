// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	log = logf.Log.WithName("resource")
)

// ResourceReporter aggregates resources of all Elastic components managed by the operator
// and reports them in a config map in the form of licensing information
type ResourceReporter struct {
	aggregator        Aggregator
	licensingResolver LicensingResolver
}

// NewResourceReporter returns a new ResourceReporter
func NewResourceReporter(client client.Client) ResourceReporter {
	c := k8s.WrapClient(client)
	return ResourceReporter{
		aggregator: Aggregator{
			client: c,
		},
		licensingResolver: LicensingResolver{
			client: c,
		},
	}
}

// Start starts to report the licensing information repeatedly at regular intervals
func (r ResourceReporter) Start(operatorNs string, refreshPeriod time.Duration) {
	// report once as soon as possible to not wait the first tick with a retry
	// because the cache may not be started
	doWithRetry(3, func() error {
		return r.Report(operatorNs)
	})

	ticker := time.NewTicker(refreshPeriod)
	for range ticker.C {
		err := r.Report(operatorNs)
		if err != nil {
			log.Error(err, "Failed to report licensing information")
		}
	}
}

func doWithRetry(maxRetries int, f func() error) {
	retry := 1
	for {
		err := f()
		if err == nil || retry == maxRetries {
			break
		}
		time.Sleep(1*time.Second)
		retry++
	}
}

// Report reports the licensing information in a config map
func (r ResourceReporter) Report(operatorNs string) error {
	licensingInfo, err := r.Get()
	if err != nil {
		return err
	}

	return r.licensingResolver.Save(licensingInfo, operatorNs)
}

// Get aggregates managed resources and returns the licensing information
func (r ResourceReporter) Get() (LicensingInfo, error) {
	totalMemory, err := r.aggregator.AggregateMemory()
	if err != nil {
		return LicensingInfo{}, err
	}

	return r.licensingResolver.ToInfo(totalMemory)
}
