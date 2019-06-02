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

func ParseEnterpriseLicenses(raw map[string][]byte) ([]SourceEnterpriseLicense, error) {
	var licenses []SourceEnterpriseLicense
	for k, v := range raw {
		var license SourceEnterpriseLicense
		err := json.Unmarshal(v, &license)
		if err != nil {
			return nil, errors.Wrapf(err, "License %s cannot be unmarshalled", k)
		}
		licenses = append(licenses, license)
	}
	return licenses, nil
}
