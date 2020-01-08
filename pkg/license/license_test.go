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
	i := LicensingInfo{}
	data, err := i.toMap()
	assert.NoError(t, err)
	assert.Equal(t, 5, len(data))
	assert.Equal(t, "", data["eck_license_level"])

	i = LicensingInfo{EckLicenseLevel: "basic"}
	data, err = i.toMap()
	assert.NoError(t, err)
	assert.Equal(t, 5, len(data))
	assert.Equal(t, "basic", data["eck_license_level"])
}

func TestMaxEnterpriseResourceUnits(t *testing.T) {
	r := LicensingResolver{}

	maxERUs := r.getMaxEnterpriseResourceUnits(nil)
	assert.Equal(t, 0, maxERUs)

	maxERUs = r.getMaxEnterpriseResourceUnits(&commonlicense.EnterpriseLicense{
		License: commonlicense.LicenseSpec{
			MaxResourceUnits: 42,
		},
	})
	assert.Equal(t, 42, maxERUs)

	maxERUs = r.getMaxEnterpriseResourceUnits(&commonlicense.EnterpriseLicense{
		License: commonlicense.LicenseSpec{
			MaxInstances: 10,
		},
	})
	assert.Equal(t, 5, maxERUs)
}
