// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	namespace          = "elastic"
	LeaderKey          = "leader"
	licensingSubsystem = "licensing"

	LicenseLevelLabel      = "license_level"
	OperatorNamespaceLabel = "operator_namespace"
	UuidLabel              = "uuid"
)

var (
	Leader = registerGauge(prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: namespace,
		Name:      LeaderKey,
		Help:      "Gauge used to evaluate if an instance is elected",
	}, []string{UuidLabel, OperatorNamespaceLabel}))

	// LicensingMaxERUGauge reports the maximum allowed enterprise resource units for licensing purposes.
	LicensingMaxERUGauge = registerGauge(prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: licensingSubsystem,
		Name:      "enterprise_resource_units_max",
		Help:      "Maximum number of enterprise resource units available",
	}, []string{LicenseLevelLabel}))

	// LicensingTotalERUGauge reports the total enterprise resource units usage for licensing purposes.
	LicensingTotalERUGauge = registerGauge(prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: licensingSubsystem,
		Name:      "enterprise_resource_units_total",
		Help:      "Total enterprise resource units used",
	}, []string{LicenseLevelLabel}))

	// LicensingTotalMemoryGauge reports the total memory usage for licensing purposes.
	LicensingTotalMemoryGauge = registerGauge(prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: licensingSubsystem,
		Name:      "memory_gigabytes_total",
		Help:      "Total memory used in GB",
	}, []string{LicenseLevelLabel}))
)

func registerGauge(gauge *prometheus.GaugeVec) *prometheus.GaugeVec {
	err := crmetrics.Registry.Register(gauge)
	if err != nil {
		if existsErr, ok := err.(prometheus.AlreadyRegisteredError); ok {
			return existsErr.ExistingCollector.(*prometheus.GaugeVec)
		}

		panic(fmt.Errorf("failed to register gauge: %w", err))
	}

	return gauge
}
