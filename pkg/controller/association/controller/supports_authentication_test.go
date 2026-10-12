// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/enterprisesearch/v1"
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/maps/v1alpha1"
)

func TestSupportsAuthentication(t *testing.T) {
	standaloneAgent := &agentv1alpha1.Agent{Spec: agentv1alpha1.AgentSpec{Mode: agentv1alpha1.AgentStandaloneMode}}
	defaultModeAgent := &agentv1alpha1.Agent{}
	fleetAgent := &agentv1alpha1.Agent{Spec: agentv1alpha1.AgentSpec{Mode: agentv1alpha1.AgentFleetMode}}
	ems := func(version string) *emsv1alpha1.ElasticMapsServer {
		return &emsv1alpha1.ElasticMapsServer{Spec: emsv1alpha1.MapsSpec{Version: version}}
	}

	tests := []struct {
		name       string
		supports   func(commonv1.Associated, commonv1.AuthenticationMode) bool
		associated commonv1.Associated
		mode       commonv1.AuthenticationMode
		want       bool
	}{
		// Agent: PKI is supported by standalone Agents only.
		{name: "agent standalone: credentials", supports: agentSupportsAuthentication, associated: standaloneAgent, mode: commonv1.AuthenticationModeCredentials, want: true},
		{name: "agent standalone: PKI", supports: agentSupportsAuthentication, associated: standaloneAgent, mode: commonv1.AuthenticationModePKI, want: true},
		{name: "agent with default mode (standalone): PKI", supports: agentSupportsAuthentication, associated: defaultModeAgent, mode: commonv1.AuthenticationModePKI, want: true},
		{name: "agent fleet: credentials", supports: agentSupportsAuthentication, associated: fleetAgent, mode: commonv1.AuthenticationModeCredentials, want: true},
		{name: "agent fleet: PKI", supports: agentSupportsAuthentication, associated: fleetAgent, mode: commonv1.AuthenticationModePKI, want: false},
		{name: "agent: PKI for an unexpected associated type", supports: agentSupportsAuthentication, associated: &apmv1.ApmServer{}, mode: commonv1.AuthenticationModePKI, want: false},

		// Enterprise Search: always sends Basic Auth, PKI is never supported.
		{name: "enterprise search: credentials", supports: entSupportsAuthentication, associated: &entv1.EnterpriseSearch{}, mode: commonv1.AuthenticationModeCredentials, want: true},
		{name: "enterprise search: PKI", supports: entSupportsAuthentication, associated: &entv1.EnterpriseSearch{}, mode: commonv1.AuthenticationModePKI, want: false},

		// Maps: PKI requires the minimum PKI version, which includes pre-releases of that version.
		{name: "maps below the minimum version: credentials", supports: mapsSupportsAuthentication, associated: ems("9.5.6"), mode: commonv1.AuthenticationModeCredentials, want: true},
		{name: "maps below the minimum version: PKI", supports: mapsSupportsAuthentication, associated: ems("9.5.6"), mode: commonv1.AuthenticationModePKI, want: false},
		{name: "maps 8.x: PKI", supports: mapsSupportsAuthentication, associated: ems("8.19.21"), mode: commonv1.AuthenticationModePKI, want: false},
		{name: "maps at the minimum version: PKI", supports: mapsSupportsAuthentication, associated: ems("9.6.0"), mode: commonv1.AuthenticationModePKI, want: true},
		{name: "maps pre-release of the minimum version: PKI", supports: mapsSupportsAuthentication, associated: ems("9.6.0-SNAPSHOT"), mode: commonv1.AuthenticationModePKI, want: true},
		{name: "maps above the minimum version: PKI", supports: mapsSupportsAuthentication, associated: ems("9.7.1"), mode: commonv1.AuthenticationModePKI, want: true},
		{name: "maps with an unparseable version: PKI", supports: mapsSupportsAuthentication, associated: ems("not-a-version"), mode: commonv1.AuthenticationModePKI, want: false},
		{name: "maps with an unparseable version: credentials", supports: mapsSupportsAuthentication, associated: ems("not-a-version"), mode: commonv1.AuthenticationModeCredentials, want: true},
		{name: "maps: PKI for an unexpected associated type", supports: mapsSupportsAuthentication, associated: &apmv1.ApmServer{}, mode: commonv1.AuthenticationModePKI, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.supports(tt.associated, tt.mode))
		})
	}
}
