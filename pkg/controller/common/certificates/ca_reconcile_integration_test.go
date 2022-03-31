// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build integration

package certificates

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCAFromFile(t *testing.T) {
	// create fixture
	crtBytes := loadFileBytes("tls.crt")
	keyBytes := loadFileBytes("tls.key")
	certs, err := ParsePEMCerts(crtBytes)
	require.NoError(t, err)
	key, err := ParsePEMPrivateKey(keyBytes)
	require.NoError(t, err)
	goodFixture := &CA{
		PrivateKey: key,
		Cert:       certs[0],
	}

	// run tests
	type args struct {
		ca  string
		key string
	}
	tests := []struct {
		name       string
		args       args
		want       *CA
		wantErrMsg string
	}{
		{
			name: "happy path",
			args: args{
				ca:  "tls.crt",
				key: "tls.key",
			},
			want:       goodFixture,
			wantErrMsg: "",
		},
		{
			name: "corrupted crt",
			args: args{
				ca:  "corrupted.crt",
				key: "tls.key",
			},
			want:       nil,
			wantErrMsg: "Cannot parse PEM cert",
		},
		{
			name: "corrupted key",
			args: args{
				ca:  "tls.crt",
				key: "corrupted.key",
			},
			want:       nil,
			wantErrMsg: "Cannot parse private key",
		},
		{
			name: "multiple certs",
			args: args{
				ca:  "chain.crt",
				key: "tls.key",
			},
			want:       nil,
			wantErrMsg: "more than one certificate in PEM file",
		},
		{
			name: "no certs",
			args: args{
				ca:  "empty.crt",
				key: "tls.key",
			},
			want:       nil,
			wantErrMsg: "did not contain any certificates",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := ioutil.TempDir("testdata", "ca-from-file")
			defer os.RemoveAll(tempDir)
			require.NoError(t, err)
			require.NoError(t, os.Link(filepath.Join("testdata", tt.args.ca), filepath.Join(tempDir, CertFileName)))
			require.NoError(t, os.Link(filepath.Join("testdata", tt.args.key), filepath.Join(tempDir, KeyFileName)))

			got, err := BuildCAFromFile(tempDir)
			if (tt.wantErrMsg != "") != (err != nil) || err != nil && !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("Want err %v but got %v", tt.wantErrMsg, err)
			}

			assert.Equalf(t, tt.want, got, "BuildCAFromFile(%+v)", tt.args)
		})
	}
}
