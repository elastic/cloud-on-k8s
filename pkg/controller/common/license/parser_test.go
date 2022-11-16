// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"os"
	"strings"
	"testing"

	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
)

func TestParseEnterpriseLicenses(t *testing.T) {
	good, err := os.ReadFile("testdata/test-license.json")
	require.NoError(t, err)
	bad, err := os.ReadFile("testdata/test-error.json")
	require.NoError(t, err)
	platinum, err := os.ReadFile("testdata/wrong-type.json")
	require.NoError(t, err)

	type args struct {
		raw map[string][]byte
	}
	tests := []struct {
		name    string
		args    args
		want    EnterpriseLicense
		wantErr string
	}{
		{
			name: "single license",
			args: args{
				raw: map[string][]byte{
					FileName: good,
				},
			},
			want: expectedLicenseSpec,
		},
		{
			name: "malformed license",
			args: args{
				raw: map[string][]byte{
					FileName: bad,
				},
			},
			wantErr: "license cannot be unmarshalled:",
		},
		{
			name: "different key",
			args: args{
				raw: map[string][]byte{
					"_": good,
				},
			},
			want: expectedLicenseSpec,
		},
		{
			name: "wrong type",
			args: args{
				raw: map[string][]byte{
					FileName: platinum,
				},
			},
			want: EnterpriseLicense{
				License: LicenseSpec{
					UID:                "57E312E2-6EA0-49D0-8E65-AA5017742ACF",
					IssueDateInMillis:  1548115200000,
					ExpiryDateInMillis: 1561247999999,
					IssuedTo:           "test org",
					Issuer:             "test issuer",
					StartDateInMillis:  1548115200000,
					Type:               "platinum",
					Signature:          "test signature platinum",
				},
			},
			wantErr: "is not an enterprise license",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEnterpriseLicense(tt.args.raw)
			if (err != nil) != (len(tt.wantErr) > 0) && strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ParseEnterpriseLicense() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := deep.Equal(got, tt.want); diff != nil {
				t.Error(diff)
			}
		})
	}
}
