/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package license

import (
	"io/ioutil"
	"reflect"
	"testing"

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
		want    []EnterpriseLicense
		wantErr bool
	}{
		{
			name: "single license",
			args: args{
				raw: map[string][]byte{
					"_": good,
				},
			},
			want: []EnterpriseLicense{
				expectedLicenseSpec,
			},
			wantErr: false,
		},
		{
			name: "multiple licenses",
			args: args{
				raw: map[string][]byte{
					"1": good,
					"2": good,
				},
			},
			want: []EnterpriseLicense{
				expectedLicenseSpec,
				expectedLicenseSpec,
			},
			wantErr: false,
		},
		{
			name: "malformed license",
			args: args{
				raw: map[string][]byte{
					"_": bad,
				},
			},
			wantErr: true,
		},
		{
			name: "mixed good/bad: all or nothing",
			args: args{
				raw: map[string][]byte{
					"1": good,
					"2": bad,
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEnterpriseLicenses(tt.args.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEnterpriseLicenses() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseEnterpriseLicenses() = %v, want %v", got, tt.want)
			}
		})
	}
}
