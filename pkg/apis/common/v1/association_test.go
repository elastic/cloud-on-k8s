// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAssociationConfIsConfigured(t *testing.T) {
	tests := []struct {
		name      string
		assocConf *AssociationConf
		want      bool
	}{
		{
			name: "nil object",
			want: false,
		},
		{
			name: "missing URL",
			assocConf: &AssociationConf{
				AuthSecretName: "auth-secret",
				AuthSecretKey:  "elastic",
				CACertProvided: true,
				CASecretName:   "ca-secret",
			},
			want: false,
		},
		{
			name: "missing auth secret name",
			assocConf: &AssociationConf{
				AuthSecretKey:  "elastic",
				CACertProvided: true,
				CASecretName:   "ca-secret",
				URL:            "https://my-es.svc",
			},
			want: false,
		},
		{
			name: "missing auth secret key",
			assocConf: &AssociationConf{
				AuthSecretName: "auth-secret",
				CACertProvided: true,
				CASecretName:   "ca-secret",
				URL:            "https://my-es.svc",
			},
			want: false,
		},
		{
			name: "missing CA secret name",
			assocConf: &AssociationConf{
				AuthSecretName: "auth-secret",
				AuthSecretKey:  "elastic",
				CACertProvided: true,
				URL:            "https://my-es.svc",
			},
			want: false,
		},
		{
			name: "correctly configured without CA",
			assocConf: &AssociationConf{
				AuthSecretName: "auth-secret",
				AuthSecretKey:  "elastic",
				CACertProvided: false,
				URL:            "https://my-es.svc",
			},
			want: true,
		},
		{
			name: "correctly configured",
			assocConf: &AssociationConf{
				AuthSecretName: "auth-secret",
				AuthSecretKey:  "elastic",
				CASecretName:   "ca-secret",
				URL:            "https://my-es.svc",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.assocConf.IsConfigured()
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFormatNameWithID(t *testing.T) {
	for _, tt := range []struct {
		name     string
		template string
		id       string
		wanted   string
	}{
		{
			name:     "no id",
			template: "association.k8s.elastic.co/es-conf%s",
			id:       "",
			wanted:   "association.k8s.elastic.co/es-conf",
		},
		{
			name:     "id present",
			template: "association.k8s.elastic.co/es-conf%s",
			id:       "agentNamespace.agentName",
			wanted:   "association.k8s.elastic.co/es-conf-agentNamespace.agentName",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wanted, FormatNameWithID(tt.template, tt.id))
		})
	}
}

func TestAssociationStatusMap_AllEstablished(t *testing.T) {
	for _, tt := range []struct {
		name      string
		statusMap AssociationStatusMap
		wanted    bool
	}{
		{
			name:      "no elements",
			statusMap: AssociationStatusMap{},
			wanted:    true,
		},
		{
			name: "single established",
			statusMap: map[string]AssociationStatus{
				"": AssociationEstablished,
			},
			wanted: true,
		},
		{
			name: "many established",
			statusMap: map[string]AssociationStatus{
				"1": AssociationEstablished,
				"2": AssociationEstablished,
			},
			wanted: true,
		},
		{
			name: "one pending",
			statusMap: map[string]AssociationStatus{
				"1": AssociationEstablished,
				"2": AssociationPending,
			},
			wanted: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wanted, tt.statusMap.AllEstablished())
		})
	}
}

func TestAssociationStatusMap_Single(t *testing.T) {
	for _, tt := range []struct {
		name         string
		statusMap    AssociationStatusMap
		wantedStatus AssociationStatus
		wantedErr    bool
	}{
		{
			name:         "no elements",
			statusMap:    AssociationStatusMap{},
			wantedStatus: AssociationUnknown,
		},
		{
			name: "single established",
			statusMap: map[string]AssociationStatus{
				"": AssociationEstablished,
			},
			wantedStatus: AssociationEstablished,
		},
		{
			name: "single pending",
			statusMap: map[string]AssociationStatus{
				"": AssociationPending,
			},
			wantedStatus: AssociationPending,
		},
		{
			name: "many established",
			statusMap: map[string]AssociationStatus{
				"1": AssociationEstablished,
				"2": AssociationEstablished,
			},
			wantedStatus: AssociationUnknown,
			wantedErr:    true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotErr := tt.statusMap.Single()
			require.Equal(t, tt.wantedStatus, gotStatus)
			require.Equal(t, tt.wantedErr, gotErr != nil)
		})
	}
}

func TestAssociationStatusMap_String(t *testing.T) {
	for _, tt := range []struct {
		name      string
		statusMap AssociationStatusMap
		wanted    string
	}{
		{
			name:      "no elements",
			statusMap: AssociationStatusMap{},
			wanted:    "",
		},
		{
			name:      "single Established status, old behavior",
			statusMap: NewSingleAssociationStatusMap(AssociationEstablished),
			wanted:    "Established",
		},
		{
			name:      "single Unknown status, old behavior",
			statusMap: NewSingleAssociationStatusMap(AssociationUnknown),
			wanted:    "",
		},
		{
			name: "single established",
			statusMap: map[string]AssociationStatus{
				"ns/name": AssociationEstablished,
			},
			wanted: "ns/name: Established",
		},
		{
			name: "single unknown",
			statusMap: map[string]AssociationStatus{
				"ns/name": AssociationUnknown,
			},
			wanted: "ns/name: ",
		},
		{
			name: "multiple mixed",
			statusMap: map[string]AssociationStatus{
				"ns/name":   AssociationEstablished,
				"ns2/name2": AssociationPending,
				"ns3/name3": AssociationFailed,
				"ns4/name4": AssociationUnknown,
			},
			wanted: "ns/name: Established, ns2/name2: Pending, ns3/name3: Failed, ns4/name4: ",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wanted, tt.statusMap.String())
		})
	}
}
