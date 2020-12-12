// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_checkSpec(t *testing.T) {
	tests := []struct {
		name    string
		beat    Agent
		wantErr bool
	}{
		{
			name: "deployment absent, dset present",
			beat: Agent{
				Spec: AgentSpec{
					DaemonSet: &DaemonSetSpec{},
				},
			},
			wantErr: false,
		},
		{
			name: "deployment present, dset absent",
			beat: Agent{
				Spec: AgentSpec{
					Deployment: &DeploymentSpec{},
				},
			},
			wantErr: false,
		},
		{
			name: "neither present",
			beat: Agent{
				Spec: AgentSpec{},
			},
			wantErr: true,
		},
		{
			name: "both present",
			beat: Agent{
				Spec: AgentSpec{
					Deployment: &DeploymentSpec{},
					DaemonSet:  &DaemonSetSpec{},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkSpec(&tc.beat)
			assert.Equal(t, tc.wantErr, len(got) > 0)
		})
	}
}
