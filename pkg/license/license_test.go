// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	commonlicense "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
)

func TestToMap(t *testing.T) {
	dateFixture := time.Date(2021, 11, 03, 0, 0, 0, 0, time.UTC)

	t.Run("empty_object", func(t *testing.T) {
		i := LicensingInfo{}
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
			Timestamp:                  "2020-05-28T11:15:31Z",
			EckLicenseLevel:            "enterprise",
			EckLicenseExpiryDate:       &dateFixture,
			TotalManagedMemoryGiB:      64,
			TotalManagedMemoryBytes:    68719476736,
			EnterpriseResourceUnits:    1,
			MaxEnterpriseResourceUnits: 10,
		}

		have := i.toMap()
		want := map[string]string{
			"timestamp":                     "2020-05-28T11:15:31Z",
			"eck_license_level":             "enterprise",
			"eck_license_expiry_date":       "2021-11-03T00:00:00Z",
			"total_managed_memory":          "64.00GiB",
			"total_managed_memory_bytes":    "68719476736",
			"enterprise_resource_units":     "1",
			"max_enterprise_resource_units": "10",
		}
		assert.Equal(t, want, have)
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
