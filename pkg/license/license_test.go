// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
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
			memoryUsage: memoryUsage{
				appUsage: map[string]managedMemory{
					elasticsearchKey: newManagedMemory(21474836480, elasticsearchKey),
					kibanaKey:        newManagedMemory(8589934592, kibanaKey),
					apmKey:           newManagedMemory(4294967296, apmKey),
					entSearchKey:     newManagedMemory(17179869184, entSearchKey),
					logstashKey:      newManagedMemory(17179869184, logstashKey),
				},
				totalMemory: newManagedMemory(68719476736, totalKey),
			},
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
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      LicensingCfgMapName,
				Labels: map[string]string{
					commonv1.TypeLabelName: Type,
				},
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
