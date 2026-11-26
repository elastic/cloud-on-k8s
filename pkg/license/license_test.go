// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/metrics"
)

func TestToMap(t *testing.T) {
	dateFixture := time.Date(2021, 11, 0o3, 0, 0, 0, 0, time.UTC)

	t.Run("empty_object", func(t *testing.T) {
		i := LicensingInfo{memoryUsage: newMemoryUsage()}
		have := i.toMap()
		want := map[string]string{
			"timestamp":                  "",
			"eck_license_level":          "",
			"total_managed_memory":       "0.00GiB",
			"total_managed_memory_bytes": "0",
			"enterprise_resource_units":  "0",
		}
		assert.Equal(t, want, have)
	})

	t.Run("complete_object", func(t *testing.T) {
		i := LicensingInfo{
			appUsage: map[string]managedMemory{
				elasticsearchKey: newManagedMemory(21474836480, elasticsearchKey),
				kibanaKey:        newManagedMemory(8589934592, kibanaKey),
				apmKey:           newManagedMemory(4294967296, apmKey),
				entSearchKey:     newManagedMemory(17179869184, entSearchKey),
				logstashKey:      newManagedMemory(17179869184, logstashKey),
			},
			totalMemory:                newManagedMemory(68719476736, totalKey),
			Timestamp:                  "2020-05-28T11:15:31Z",
			EckLicenseLevel:            "enterprise",
			EckLicenseExpiryDate:       &dateFixture,
			EnterpriseResourceUnits:    1,
			MaxEnterpriseResourceUnits: 10,
		}

		have := i.toMap()
		want := map[string]string{
			"timestamp":                      "2020-05-28T11:15:31Z",
			"eck_license_level":              "enterprise",
			"eck_license_expiry_date":        "2021-11-03T00:00:00Z",
			"elasticsearch_memory":           "20.00GiB",
			"elasticsearch_memory_bytes":     "21474836480",
			"kibana_memory":                  "8.00GiB",
			"kibana_memory_bytes":            "8589934592",
			"apm_memory":                     "4.00GiB",
			"apm_memory_bytes":               "4294967296",
			"enterprise_search_memory":       "16.00GiB",
			"enterprise_search_memory_bytes": "17179869184",
			"logstash_memory":                "16.00GiB",
			"logstash_memory_bytes":          "17179869184",
			"total_managed_memory":           "64.00GiB",
			"total_managed_memory_bytes":     "68719476736",
			"enterprise_resource_units":      "1",
			"max_enterprise_resource_units":  "10",
		}
		assert.Equal(t, want, have)
	})
}

func TestReportAsMetrics(t *testing.T) {
	// The gauges are registered in a global registry shared by all tests, reset the licensing
	// gauges and their state to avoid leaking series to other tests.
	resetLicenseMetricsForTest()
	defer resetLicenseMetricsForTest()

	t.Run("sets the expiry timestamp when the license has an expiry date", func(t *testing.T) {
		resetLicenseMetricsForTest()
		expiry := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
		info := LicensingInfo{
			memoryUsage:          newMemoryUsage(),
			EckLicenseLevel:      "enterprise",
			EckLicenseExpiryDate: &expiry,
		}

		info.ReportAsMetrics()

		assert.Equal(t,
			float64(expiry.Unix()),
			testutil.ToFloat64(metrics.LicensingExpiryTimestampGauge.WithLabelValues("enterprise")),
		)
	})

	t.Run("does not set the expiry timestamp when the license has no expiry date", func(t *testing.T) {
		resetLicenseMetricsForTest()
		// Report an expiring license first to ensure its series is removed.
		expiry := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
		LicensingInfo{
			memoryUsage:          newMemoryUsage(),
			EckLicenseLevel:      "enterprise-trial",
			EckLicenseExpiryDate: &expiry,
		}.ReportAsMetrics()
		require.Equal(t, 1, testutil.CollectAndCount(metrics.LicensingExpiryTimestampGauge))

		LicensingInfo{
			memoryUsage:     newMemoryUsage(),
			EckLicenseLevel: defaultOperatorLicenseLevel,
		}.ReportAsMetrics()

		assert.Equal(t, 0, testutil.CollectAndCount(metrics.LicensingExpiryTimestampGauge))
	})

	t.Run("removes the previous series when the license level changes", func(t *testing.T) {
		resetLicenseMetricsForTest()
		trialExpiry := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
		LicensingInfo{
			memoryUsage:          newMemoryUsage(),
			EckLicenseLevel:      "enterprise-trial",
			EckLicenseExpiryDate: &trialExpiry,
		}.ReportAsMetrics()
		require.Equal(t, 1, testutil.CollectAndCount(metrics.LicensingExpiryTimestampGauge))

		enterpriseExpiry := time.Date(2027, 11, 3, 0, 0, 0, 0, time.UTC)
		LicensingInfo{
			memoryUsage:          newMemoryUsage(),
			EckLicenseLevel:      "enterprise",
			EckLicenseExpiryDate: &enterpriseExpiry,
		}.ReportAsMetrics()

		assert.Equal(t, 1, testutil.CollectAndCount(metrics.LicensingExpiryTimestampGauge))
		assert.Equal(t,
			float64(enterpriseExpiry.Unix()),
			testutil.ToFloat64(metrics.LicensingExpiryTimestampGauge.WithLabelValues("enterprise")),
		)
	})

	t.Run("removes all licensing series when the license level changes", func(t *testing.T) {
		resetLicenseMetricsForTest()
		memoryGauges := licensingMemoryGauges()
		expiry := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
		LicensingInfo{
			memoryUsage:                newMemoryUsage(),
			EckLicenseLevel:            "enterprise",
			EckLicenseExpiryDate:       &expiry,
			EnterpriseResourceUnits:    5,
			MaxEnterpriseResourceUnits: 10,
		}.ReportAsMetrics()

		for _, gauge := range memoryGauges {
			requireLicenseLevelSeries(t, gauge, "enterprise")
		}
		requireLicenseLevelSeries(t, metrics.LicensingTotalERUGauge, "enterprise")
		requireLicenseLevelSeries(t, metrics.LicensingMaxERUGauge, "enterprise")
		requireLicenseLevelSeries(t, metrics.LicensingExpiryTimestampGauge, "enterprise")

		LicensingInfo{
			memoryUsage:     newMemoryUsage(),
			EckLicenseLevel: defaultOperatorLicenseLevel,
		}.ReportAsMetrics()

		// These gauges are also reported for the basic license, so only the stale enterprise series
		// must have been removed.
		for _, gauge := range memoryGauges {
			requireLicenseLevelSeries(t, gauge, defaultOperatorLicenseLevel)
		}
		requireLicenseLevelSeries(t, metrics.LicensingTotalERUGauge, defaultOperatorLicenseLevel)
		// A basic license has neither a maximum ERU nor an expiry date.
		assert.Equal(t, 0, testutil.CollectAndCount(metrics.LicensingMaxERUGauge))
		assert.Equal(t, 0, testutil.CollectAndCount(metrics.LicensingExpiryTimestampGauge))
	})

	t.Run("removes the max ERU series when it disappears at the same license level", func(t *testing.T) {
		resetLicenseMetricsForTest()
		LicensingInfo{
			memoryUsage:                newMemoryUsage(),
			EckLicenseLevel:            "enterprise",
			MaxEnterpriseResourceUnits: 10,
		}.ReportAsMetrics()
		require.Equal(t, 1, testutil.CollectAndCount(metrics.LicensingMaxERUGauge))

		LicensingInfo{
			memoryUsage:     newMemoryUsage(),
			EckLicenseLevel: "enterprise",
		}.ReportAsMetrics()

		assert.Equal(t, 0, testutil.CollectAndCount(metrics.LicensingMaxERUGauge))
	})

	t.Run("removes a series published with an empty license level", func(t *testing.T) {
		resetLicenseMetricsForTest()
		expiry := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
		LicensingInfo{
			memoryUsage:          newMemoryUsage(),
			EckLicenseExpiryDate: &expiry,
		}.ReportAsMetrics()
		require.Equal(t, 1, testutil.CollectAndCount(metrics.LicensingExpiryTimestampGauge))

		LicensingInfo{
			memoryUsage:     newMemoryUsage(),
			EckLicenseLevel: defaultOperatorLicenseLevel,
		}.ReportAsMetrics()

		assert.Equal(t, 0, testutil.CollectAndCount(metrics.LicensingExpiryTimestampGauge))
	})

	t.Run("keeps the series when reporting the same license level", func(t *testing.T) {
		resetLicenseMetricsForTest()
		firstExpiry := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
		LicensingInfo{
			memoryUsage:          newMemoryUsage(),
			EckLicenseLevel:      "enterprise",
			EckLicenseExpiryDate: &firstExpiry,
		}.ReportAsMetrics()
		require.Equal(t, 1, testutil.CollectAndCount(metrics.LicensingExpiryTimestampGauge))
		// Keep a reference to the gauge created by the first report: if a later report resets and
		// recreates the series, this gauge is no longer updated and the assertion below fails.
		gauge := metrics.LicensingExpiryTimestampGauge.WithLabelValues("enterprise")
		require.Equal(t, float64(firstExpiry.Unix()), testutil.ToFloat64(gauge))

		secondExpiry := time.Date(2027, 11, 3, 0, 0, 0, 0, time.UTC)
		LicensingInfo{
			memoryUsage:          newMemoryUsage(),
			EckLicenseLevel:      "enterprise",
			EckLicenseExpiryDate: &secondExpiry,
		}.ReportAsMetrics()

		assert.Equal(t, 1, testutil.CollectAndCount(metrics.LicensingExpiryTimestampGauge))
		assert.Equal(t, float64(secondExpiry.Unix()), testutil.ToFloat64(gauge))
	})

	t.Run("concurrent reports leave a complete set of metrics for the surviving license level", func(t *testing.T) {
		resetLicenseMetricsForTest()
		defer resetLicenseMetricsForTest()

		expiry := time.Date(2027, 11, 3, 0, 0, 0, 0, time.UTC)
		enterprise := LicensingInfo{
			memoryUsage:                testMemoryUsage(64, 32),
			EckLicenseLevel:            "enterprise",
			EckLicenseExpiryDate:       &expiry,
			EnterpriseResourceUnits:    5,
			MaxEnterpriseResourceUnits: 10,
		}
		basic := LicensingInfo{
			memoryUsage:     testMemoryUsage(32, 16),
			EckLicenseLevel: defaultOperatorLicenseLevel,
		}

		// Publish and clean up at the same time: if the lock only covered cleanup, one report could
		// delete the series another report has just published for its license level.
		const reportsPerLevel = 100
		start := make(chan struct{})
		var wg sync.WaitGroup
		for range reportsPerLevel {
			for _, info := range []LicensingInfo{enterprise, basic} {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					info.ReportAsMetrics()
				}()
			}
		}
		close(start)
		wg.Wait()

		levels := map[string]struct{}{}
		for _, gauge := range licensingGaugeVecs {
			for _, labels := range gaugeLabelSets(t, gauge) {
				levels[labels[metrics.LicenseLevelLabel]] = struct{}{}
			}
		}
		require.Len(t, levels, 1, "concurrent reports did not leave a single license level: %v", levels)
		var survivingLevel string
		for level := range levels {
			survivingLevel = level
		}

		// Every always-reported gauge must expose exactly one series, for the surviving level.
		for _, gauge := range licensingMemoryGauges() {
			requireLicenseLevelSeries(t, gauge, survivingLevel)
		}
		requireLicenseLevelSeries(t, metrics.LicensingTotalERUGauge, survivingLevel)

		expectedTotalMemory, expectedESMemory, expectedERUs := 64.0, 32.0, 5.0
		expectedMaxERUs, expectedExpiry := 10.0, float64(expiry.Unix())
		if survivingLevel == defaultOperatorLicenseLevel {
			expectedTotalMemory, expectedESMemory, expectedERUs = 32.0, 16.0, 0
		}
		assert.Equal(t, expectedTotalMemory, testutil.ToFloat64(metrics.LicensingTotalMemoryGauge.WithLabelValues(survivingLevel)))
		assert.Equal(t, expectedESMemory, testutil.ToFloat64(metrics.LicensingESMemoryGauge.WithLabelValues(survivingLevel)))
		assert.Equal(t, expectedERUs, testutil.ToFloat64(metrics.LicensingTotalERUGauge.WithLabelValues(survivingLevel)))

		if survivingLevel == "enterprise" {
			assert.Equal(t, expectedMaxERUs, testutil.ToFloat64(metrics.LicensingMaxERUGauge.WithLabelValues(survivingLevel)))
			assert.Equal(t, expectedExpiry, testutil.ToFloat64(metrics.LicensingExpiryTimestampGauge.WithLabelValues(survivingLevel)))
		} else {
			// A basic license has neither a maximum ERU nor an expiry date.
			assert.Equal(t, 0, testutil.CollectAndCount(metrics.LicensingMaxERUGauge))
			assert.Equal(t, 0, testutil.CollectAndCount(metrics.LicensingExpiryTimestampGauge))
		}
	})
}

// licensingMemoryGauges returns the per-component memory gauges. This is an explicit list instead of
// licensingGaugeVecs so that a gauge missing from the stale series cleanup is caught by the tests.
func licensingMemoryGauges() []*prometheus.GaugeVec {
	return []*prometheus.GaugeVec{
		metrics.LicensingTotalMemoryGauge,
		metrics.LicensingESMemoryGauge,
		metrics.LicensingKBMemoryGauge,
		metrics.LicensingAPMMemoryGauge,
		metrics.LicensingEntSearchMemoryGauge,
		metrics.LicensingLogstashMemoryGauge,
		metrics.LicensingFleetServerMemoryGauge,
	}
}

// testMemoryUsage returns a memory usage with the given total and Elasticsearch memory in GiB.
func testMemoryUsage(totalGiB, elasticsearchGiB int64) memoryUsage {
	mu := newMemoryUsage()
	mu.totalMemory = newManagedMemory(totalGiB*GiB, totalKey)
	mu.appUsage[elasticsearchKey] = newManagedMemory(elasticsearchGiB*GiB, elasticsearchKey)
	return mu
}

// gaugeLabelSets returns the label sets of the series currently published by gauge.
func gaugeLabelSets(t *testing.T, gauge *prometheus.GaugeVec) []map[string]string {
	t.Helper()
	registry := prometheus.NewPedanticRegistry()
	require.NoError(t, registry.Register(gauge))
	families, err := registry.Gather()
	require.NoError(t, err)

	var labelSets []map[string]string
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			labels := make(map[string]string, len(metric.GetLabel()))
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			labelSets = append(labelSets, labels)
		}
	}
	return labelSets
}

// requireLicenseLevelSeries asserts that gauge publishes exactly one series with the given license level.
func requireLicenseLevelSeries(t *testing.T, gauge *prometheus.GaugeVec, level string) {
	t.Helper()
	require.Equal(t, []map[string]string{{metrics.LicenseLevelLabel: level}}, gaugeLabelSets(t, gauge))
}

func resetLicenseMetricsForTest() {
	lastReportedLicenseLevel.Lock()
	defer lastReportedLicenseLevel.Unlock()
	lastReportedLicenseLevel.level = ""
	lastReportedLicenseLevel.reported = false
	for _, gauge := range licensingGaugeVecs {
		gauge.Reset()
	}
}

func TestLicensingResolver_Save(t *testing.T) {
	const ns = "elastic-system"
	cmKey := types.NamespacedName{Namespace: ns, Name: LicensingCfgMapName}

	info := LicensingInfo{
		memoryUsage:             newMemoryUsage(),
		Timestamp:               "2026-04-28T00:00:00Z",
		EckLicenseLevel:         "basic",
		EnterpriseResourceUnits: 0,
	}

	wantLabels := map[string]string{
		commonv1.TypeLabelName:                     Type,
		commonv1.RestrictWatchedResourcesLabelName: commonv1.RestrictWatchedResourcesLabelValue,
	}

	t.Run("creates config map with watch label when it does not exist", func(t *testing.T) {
		client := k8s.NewFakeClient()
		r := LicensingResolver{operatorNs: ns, client: client}

		require.NoError(t, r.Save(context.Background(), info))

		var got corev1.ConfigMap
		require.NoError(t, client.Get(context.Background(), cmKey, &got))
		assert.Equal(t, wantLabels, got.Labels)
	})

	t.Run("adds watch label when config map already exists with stale data", func(t *testing.T) {
		// pre-existing config map carrying only the legacy type label and stale data,
		// so that NeedsUpdate triggers and labels get reconciled to the expected set.
		existing := &corev1.ConfigMap{
			Namespace: ns,
			Name:      LicensingCfgMapName,
			Labels: map[string]string{
				commonv1.TypeLabelName: Type,
			},
			Data: map[string]string{
				"eck_license_level": "stale",
			},
		}
		client := k8s.NewFakeClient(existing)
		r := LicensingResolver{operatorNs: ns, client: client}

		require.NoError(t, r.Save(context.Background(), info))

		var got corev1.ConfigMap
		require.NoError(t, client.Get(context.Background(), cmKey, &got))
		assert.Equal(t, wantLabels, got.Labels)
	})
}

func TestMaxEnterpriseResourceUnits(t *testing.T) {
	r := LicensingResolver{}

	maxERUs := r.getMaxEnterpriseResourceUnits(nil)
	assert.EqualValues(t, 0, maxERUs)

	maxERUs = r.getMaxEnterpriseResourceUnits(&commonlicense.EnterpriseLicense{
		License: commonlicense.LicenseSpec{
			MaxResourceUnits: 42,
		},
	})
	assert.EqualValues(t, 42, maxERUs)

	maxERUs = r.getMaxEnterpriseResourceUnits(&commonlicense.EnterpriseLicense{
		License: commonlicense.LicenseSpec{
			MaxInstances: 10,
		},
	})
	assert.EqualValues(t, 5, maxERUs)
}
