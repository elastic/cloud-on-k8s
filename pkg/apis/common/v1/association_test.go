// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
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
			template: "name%s",
			id:       "",
			wanted:   "name",
		},
		{
			name:     "id present",
			template: "name%s",
			id:       "ns1-es1",
			wanted:   "name-ns1-es1",
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
