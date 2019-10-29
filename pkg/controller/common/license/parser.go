// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"encoding/json"

	"github.com/pkg/errors"
)

func ParseEnterpriseLicense(raw map[string][]byte) (EnterpriseLicense, error) {
	var license EnterpriseLicense

	bytes, err := FetchLicenseData(raw)
	if err != nil {
		return license, err
	}

	err = json.Unmarshal(bytes, &license)
	if err != nil {
		return license, errors.Wrapf(err, "license cannot be unmarshalled")
	}

	return license, nil
}

func FetchLicenseData(raw map[string][]byte) ([]byte, error) {
	if len(raw) != 1 {
		return nil, errors.New("license secret needs to contain exactly one file with any name")
	}

	var result []byte
	// will only loop once due to the check above
	for _, bytes := range raw {
		result = bytes
	}

	return result, nil
}
