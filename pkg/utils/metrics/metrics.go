// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package metrics

import (
	"errors"
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
	UUIDLabel              = "uuid"
)

var (
	Leader = registerGauge(prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: namespace,
		Name:      LeaderKey,
		Help:      "Gauge used to evaluate if an instance is elected",
	}, []string{UUIDLabel, OperatorNamespaceLabel}))

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
		Name:      "memory_gibibytes_total",
		Help:      "Total memory used in GiB",
	}, []string{LicenseLevelLabel}))
)

func registerGauge(gauge *prometheus.GaugeVec) *prometheus.GaugeVec {
	err := crmetrics.Registry.Register(gauge)
	if err != nil {
		existsErr := new(prometheus.AlreadyRegisteredError)
		if errors.As(err, &existsErr) {
			return existsErr.ExistingCollector.(*prometheus.GaugeVec) //nolint:forcetypeassert
		}

		panic(fmt.Errorf("failed to register gauge: %w", err))
	}

	return gauge
}
