// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNamers(t *testing.T) {
	tests := []struct {
		name  string
		namer func(string) string
		arg   string
		want  string
	}{
		{
			name:  "test httpService namer",
			namer: HTTPService,
			arg:   "sample",
			want:  "sample-kb-http",
		},
		{
			name:  "test deployment namer",
			namer: Deployment,
			arg:   "sample",
			want:  "sample-kb",
		},
		{
			name:  "test scripts configmap namer",
			namer: ScriptsConfigMap,
			arg:   "sample",
			want:  "sample-kb-scripts",
		},
		{
			name:  "test ConfigSecret namer",
			namer: ConfigSecret,
			arg:   "sample",
			want:  "sample-kb-config",
		},
		{
			name:  "test BackgroundTasksDeployment namer",
			namer: BackgroundTasksDeployment,
			arg:   "sample",
			want:  "sample-kb-bg",
		},
		{
			name:  "test BackgroundTasksConfigSecret namer",
			namer: BackgroundTasksConfigSecret,
			arg:   "sample",
			want:  "sample-kb-bg-config",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.namer(tt.arg); got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestBackgroundTasksNameLengthBudget asserts that a Kibana name at the maximum allowed length
// (MaxResourceNameLength = 36 characters) produces resource names within Kubernetes limits.
// The longest new resource is the background tasks config secret: "<name>-kb-bg-config".
// Kubernetes label values are capped at 63 characters; Kubernetes DNS names at 253 characters.
func TestBackgroundTasksNameLengthBudget(t *testing.T) {
	// MaxResourceNameLength is 36; fill to that limit.
	maxName := strings.Repeat("x", 36)

	bgDeployment := BackgroundTasksDeployment(maxName)
	assert.LessOrEqual(t, len(bgDeployment), 63,
		"BackgroundTasksDeployment must fit in 63 characters for use as a label value")

	bgConfig := BackgroundTasksConfigSecret(maxName)
	assert.LessOrEqual(t, len(bgConfig), 63,
		"BackgroundTasksConfigSecret must fit in 63 characters for use as a label value")
}
