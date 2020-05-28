// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"testing"

	commonlicense "github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/stretchr/testify/assert"
)

func TestToMap(t *testing.T) {
	t.Run("empty_object", func(t *testing.T) {
		i := LicensingInfo{}
		have := i.toMap()
		want := map[string]string{
			"timestamp":                 "",
			"eck_license_level":         "",
			"total_managed_memory":      "0.00GB",
			"enterprise_resource_units": "0",
		}
		assert.Equal(t, want, have)
	})

	t.Run("complete_object", func(t *testing.T) {
		i := LicensingInfo{
			Timestamp:                  "2020-05-28T11:15:31Z",
			EckLicenseLevel:            "enterprise",
			TotalManagedMemory:         72.54578,
			EnterpriseResourceUnits:    5,
			MaxEnterpriseResourceUnits: 10,
		}

		have := i.toMap()
		want := map[string]string{
			"timestamp":                     "2020-05-28T11:15:31Z",
			"eck_license_level":             "enterprise",
			"total_managed_memory":          "72.55GB",
			"enterprise_resource_units":     "5",
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
