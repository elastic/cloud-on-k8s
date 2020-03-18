// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package whitelist

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/hack/licence-detector/dependency"
	"github.com/stretchr/testify/assert"
)

func TestCheckWhitelist(t *testing.T) {
	tests := []struct {
		name     string
		direct   []dependency.Info
		indirect []dependency.Info
		wantErr  bool
	}{
		{
			name: "happy path",
			direct: []dependency.Info{
				{
					Name:        "foo",
					LicenceType: "MIT",
				},
				{
					Name:        "bar",
					LicenceType: "ISC",
				},
			},
			indirect: []dependency.Info{
				{
					Name:        "baz",
					LicenceType: "BSD-3-Clause",
				},
				{
					Name:        "qux",
					LicenceType: "Apache-2.0",
				},
			},
			wantErr: false,
		},
		{
			name: "sad direct",
			direct: []dependency.Info{
				{
					Name:        "foo",
					LicenceType: "UNKNOWN",
				},
				{
					Name:        "bar",
					LicenceType: "ISC",
				},
			},
			indirect: []dependency.Info{
				{
					Name:        "baz",
					LicenceType: "BSD-3-Clause",
				},
				{
					Name:        "qux",
					LicenceType: "Apache-2.0",
				},
			},
			wantErr: true,
		},
		{
			name: "sad indirect",
			direct: []dependency.Info{
				{
					Name:        "foo",
					LicenceType: "MIT",
				},
				{
					Name:        "bar",
					LicenceType: "ISC",
				},
			},
			indirect: []dependency.Info{
				{
					Name:        "baz",
					LicenceType: "BSD-3-Clause",
				},
				{
					Name:        "qux",
					LicenceType: "Apache-1.0",
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := &dependency.List{
				Direct:   tc.direct,
				Indirect: tc.indirect,
			}
			res := CheckWhitelist(deps)
			if tc.wantErr {
				assert.Error(t, res)
			} else {
				assert.NoError(t, res)
			}
		})
	}
}
