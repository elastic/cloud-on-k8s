/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package license

import (
	"io/ioutil"
	"testing"

	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
)

func TestParseEnterpriseLicenses(t *testing.T) {
	good, err := ioutil.ReadFile("testdata/test-license.json")
	require.NoError(t, err)
	bad, err := ioutil.ReadFile("testdata/test-error.json")
	require.NoError(t, err)

	type args struct {
		raw map[string][]byte
	}
	tests := []struct {
		name    string
		args    args
		want    EnterpriseLicense
		wantErr bool
	}{
		{
			name: "single license",
			args: args{
				raw: map[string][]byte{
					FileName: good,
				},
			},
			want:    expectedLicenseSpec,
			wantErr: false,
		},
		{
			name: "malformed license",
			args: args{
				raw: map[string][]byte{
					FileName: bad,
				},
			},
			wantErr: true,
		},
		{
			name: "wrong key",
			args: args{
				raw: map[string][]byte{
					"_": good,
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEnterpriseLicense(tt.args.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEnterpriseLicense() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := deep.Equal(got, tt.want); diff != nil {
				t.Error(diff)
			}
		})
	}
}
