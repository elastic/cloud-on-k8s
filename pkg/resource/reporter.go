// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package resource

import (
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// refreshPeriod defines how long the licensing information should be refreshed
	refreshPeriod = 2 * time.Minute
)

var (
	log = logf.Log.WithName("resource")
)

// LicensingReporter aggregates resources of all Elastic components managed by the operator
// and reports them in a config map in the form of licensing information
type LicensingReporter struct {
	aggregator      Aggregator
	licenseResolver LicensingResolver
}

// LicensingReporter returns a new LicensingReporter
func NewLicensingReporter(client client.Client) LicensingReporter {
	c := k8s.WrapClient(client)
	return LicensingReporter{
		aggregator: Aggregator{
			client: c,
		},
		licenseResolver: LicensingResolver{
			client: c,
		},
	}
}

// Start starts to report the licensing information repeatedly at regular intervals
func (r LicensingReporter) Start(operatorNs string) {
	ticker := time.NewTicker(refreshPeriod)
	for range ticker.C {
		err := r.Report(operatorNs)
		if err != nil {
			log.Error(err, "Failed to report licensing information")
		}
	}
}

// Report reports the licensing information in a config map
func (r LicensingReporter) Report(operatorNs string) error {
	licensingInfo, err := r.Get()
	if err != nil {
		return err
	}

	return r.licenseResolver.Save(licensingInfo, operatorNs)
}

// Get aggregates managed resources and returns the licensing information
func (r LicensingReporter) Get() (LicensingInfo, error) {
	totalMemory, err := r.aggregator.AggregateMemory()
	if err != nil {
		return LicensingInfo{}, err
	}

	return r.licenseResolver.ToInfo(totalMemory), nil
}
