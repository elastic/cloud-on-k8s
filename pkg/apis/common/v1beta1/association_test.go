// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1

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
				CASecretName:   "ca-secret",
			},
			want: false,
		},
		{
			name: "missing auth secret name",
			assocConf: &AssociationConf{
				AuthSecretKey: "elastic",
				CASecretName:  "ca-secret",
				URL:           "https://my-es.svc",
			},
			want: false,
		},
		{
			name: "missing auth secret key",
			assocConf: &AssociationConf{
				AuthSecretName: "auth-secret",
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
				URL:            "https://my-es.svc",
			},
			want: false,
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
