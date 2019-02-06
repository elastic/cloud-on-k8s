// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package diskutil

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createPathAndDelete(t *testing.T, p string) (string, func()) {
	if p == "" {
		return p, func() {}
	}

	createdPath, err := ioutil.TempDir("", p)
	if err != nil {
		t.Fatal(err)
	}
	return createdPath, func() {
		if err := os.RemoveAll(createdPath); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEnsureDirExists(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		err  string
	}{
		{
			name: "fails with empty path",
			args: args{},
			err:  "mkdir : no such file or directory",
		},
		{
			name: "succeeds",
			args: args{
				path: "somepath",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, cleanup := createPathAndDelete(t, tt.args.path)
			println(p)
			defer cleanup()
			if err := EnsureDirExists(p); err != nil {
				assert.Equal(t, tt.err, err.Error())
				return
			}
			if tt.err != "" {
				assert.Fail(t, "Expected error not present", tt.err)
			}
		})
	}
}
