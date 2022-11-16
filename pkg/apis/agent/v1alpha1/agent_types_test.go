// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
)

func TestAgentESAssociation_AssociationConfAnnotationName(t *testing.T) {
	for _, tt := range []struct {
		name string
		ref  commonv1.ObjectSelector
		want string
	}{
		{
			name: "average length names",
			ref:  commonv1.ObjectSelector{Namespace: "namespace1", Name: "elasticsearch1"},
			want: "association.k8s.elastic.co/es-conf-2150608354",
		},
		{
			name: "max length namespace and name (63 and 36 respectively)",
			ref: commonv1.ObjectSelector{
				Namespace: "longnamespacelongnamespacelongnamespacelongnamespacelongnamespa",
				Name:      "elasticsearch1elasticsearch1elastics"},
			want: "association.k8s.elastic.co/es-conf-3419573237",
		},
		{
			name: "secret name gives a different hash",
			ref:  commonv1.ObjectSelector{Namespace: "namespace1", SecretName: "elasticsearch1"},
			want: "association.k8s.elastic.co/es-conf-851285294",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			aea := AgentESAssociation{ref: tt.ref}
			got := aea.AssociationConfAnnotationName()

			require.Equal(t, tt.want, got)
			tokens := strings.Split(got, "/")
			require.Equal(t, 2, len(tokens))
			require.LessOrEqual(t, len(tokens[0]), 253)
			require.LessOrEqual(t, len(tokens[1]), 63)
		})
	}
}

func TestModeFunctions(t *testing.T) {
	for _, tt := range []struct {
		name               string
		modeString         string
		wantFleetMode      bool
		wantStandaloneMode bool
	}{
		{
			name:               "standalone - implicit (default)",
			modeString:         "",
			wantFleetMode:      false,
			wantStandaloneMode: true,
		},
		{
			name:               "standalone - explicit",
			modeString:         "standalone",
			wantFleetMode:      false,
			wantStandaloneMode: true,
		},
		{
			name:               "fleet - explicit",
			modeString:         "fleet",
			wantFleetMode:      true,
			wantStandaloneMode: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spec := AgentSpec{Mode: AgentMode(tt.modeString)}

			require.Equal(t, tt.wantFleetMode, spec.FleetModeEnabled())
			require.Equal(t, tt.wantStandaloneMode, spec.StandaloneModeEnabled())
		})
	}
}
