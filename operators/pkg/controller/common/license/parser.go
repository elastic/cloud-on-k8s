/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package license

import (
	"encoding/json"

	"github.com/pkg/errors"
)

func ParseEnterpriseLicense(raw map[string][]byte) (EnterpriseLicense, error) {
	var license EnterpriseLicense
	err := json.Unmarshal(raw[FileName], &license)
	if err != nil {
		return EnterpriseLicense{}, errors.Wrapf(err, "License cannot be unmarshalled")
	}
	return license, nil
}
