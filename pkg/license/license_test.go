// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToMap(t *testing.T) {
	i := LicensingInfo{}
	data, err := i.toMap()
	assert.NoError(t, err)
	assert.Equal(t, 4, len(data))
	assert.Equal(t, "", data["eck_license_level"])

	i = LicensingInfo{EckLicenseLevel: "basic"}
	data, err = i.toMap()
	assert.NoError(t, err)
	assert.Equal(t, 4, len(data))
	assert.Equal(t, "basic", data["eck_license_level"])
}
