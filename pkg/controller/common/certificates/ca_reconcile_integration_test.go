// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build integration

package certificates

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			wantErrMsg: "cannot parse PEM cert",
		},
		{
			name: "corrupted key",
			args: args{
				ca:  "tls.crt",
				key: "corrupted.key",
			},
			want:       nil,
			wantErrMsg: "cannot parse private key",
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
			wantErrMsg: "does not contain any certificates",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// creating a temp dir inside testdata so that we can simply link to the test files
			tempDir, err := os.MkdirTemp("testdata", "ca-from-file")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

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

func Test_detectCAFileNames(t *testing.T) {
	// special case directory does not exist
	_, _, err := detectCAFileNames("/does/not/exist")
	require.Error(t, err)
	require.Equal(t, "global CA directory /does/not/exist does not exist", err.Error())

	// tests based on an existing directory
	tests := []struct {
		name     string
		files    []string
		wantCert string
		wantKey  string
		wantErr  bool
	}{
		{
			name:     "happy path ca*",
			files:    []string{"ca.crt", "ca.key"},
			wantCert: "ca.crt",
			wantKey:  "ca.key",
			wantErr:  false,
		},
		{
			name:     "happy path tls*",
			files:    []string{"tls.crt", "tls.key"},
			wantCert: "tls.crt",
			wantKey:  "tls.key",
			wantErr:  false,
		},
		{
			name:     "tls.* with ca.crt OK",
			files:    []string{"tls.crt", "tls.key", "ca.crt"},
			wantCert: "tls.crt",
			wantKey:  "tls.key",
			wantErr:  false,
		},
		{
			name:     "mixed tls.* and ca.* NOK",
			files:    []string{"tls.crt", "tls.key", "ca.crt", "ca.key"},
			wantCert: "",
			wantKey:  "",
			wantErr:  true,
		},
		{
			name:     "no valid combination of files",
			files:    nil,
			wantCert: "",
			wantKey:  "",
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("contents"), 0644))
			}

			cert, key, err := detectCAFileNames(dir)
			if tt.wantErr != (err != nil) {
				t.Errorf(fmt.Sprintf("want err %v got %v,files: %v ", tt.wantErr, err, tt.files))
			}
			if err == nil {
				assert.Equalf(t, filepath.Join(dir, tt.wantCert), cert, "detectCAFileNames(), files: %v", tt.files)
				assert.Equalf(t, filepath.Join(dir, tt.wantKey), key, "detectCAFileNames(), files: %v", tt.files)
			}
		})
	}
}
