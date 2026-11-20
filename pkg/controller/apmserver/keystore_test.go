// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
)

func Test_keystoreInitContainerParameters(t *testing.T) {
	fromDirCommand := []string{
		"/usr/share/apm-server/apm-server", "keystore", "add", "--from-dir", "/mnt/elastic-internal/secure-settings", "--force",
	}
	tests := []struct {
		name        string
		version     string
		wantCommand []string
	}{
		{name: "8.18 uses the keystore script", version: "8.18.8", wantCommand: nil},
		{name: "8.19 before --from-dir uses the keystore script", version: "8.19.25", wantCommand: nil},
		{name: "8.19 with --from-dir uses the command", version: "8.19.26", wantCommand: fromDirCommand},
		{name: "8.19 snapshot with --from-dir uses the command", version: "8.19.26-SNAPSHOT", wantCommand: fromDirCommand},
		{name: "9.4 uses the keystore script", version: "9.4.10", wantCommand: nil},
		{name: "9.5 before --from-dir uses the keystore script", version: "9.5.7", wantCommand: nil},
		{name: "9.5 with --from-dir uses the command", version: "9.5.8", wantCommand: fromDirCommand},
		{name: "9.6 uses the command", version: "9.6.1", wantCommand: fromDirCommand},
		{name: "9.6 snapshot uses the command", version: "9.6.1-SNAPSHOT", wantCommand: fromDirCommand},
		{name: "10.0 uses the command", version: "10.0.0", wantCommand: fromDirCommand},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := initContainerParameters
			want.Command = tt.wantCommand
			assert.Equal(t, want, keystoreInitContainerParameters(version.MustParse(tt.version)))
		})
	}
}
