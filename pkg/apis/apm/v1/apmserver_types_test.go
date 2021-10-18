// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApmEsAssociation_AssociationConfAnnotationName(t *testing.T) {
	aea := ApmEsAssociation{}
	require.Equal(t, "association.k8s.elastic.co/es-conf", aea.AssociationConfAnnotationName())
}

func TestApmKibanaAssociation_AssociationConfAnnotationName(t *testing.T) {
	aka := ApmKibanaAssociation{}
	require.Equal(t, "association.k8s.elastic.co/kb-conf", aka.AssociationConfAnnotationName())
}

func TestEffectiveVersion(t *testing.T) {
	for _, tt := range []struct {
		name        string
		version     string
		wantVersion string
	}{
		{
			name:        "no suffix",
			version:     "7.16.0",
			wantVersion: "7.16.0",
		},
		{
			name:        "prerelase suffix",
			version:     "7.16.0-alpha1",
			wantVersion: "7.16.0",
		},
		{
			name:        "SNAPSHOT prerelease suffix",
			version:     "8.0.0-SNAPSHOT",
			wantVersion: "8.0.0",
		},
		{
			name:        "build suffix",
			version:     "8.0.0+3fae3fc",
			wantVersion: "8.0.0",
		},
		{
			name:        "build and prerelease suffix",
			version:     "8.0.0+3fae3fc-SNAPSHOT",
			wantVersion: "8.0.0",
		},
		{
			name:        "malformed version",
			version:     "7.8.bad",
			wantVersion: "7.8.bad",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			apm := ApmServer{Spec: ApmServerSpec{Version: tt.version}}
			gotVersion := apm.EffectiveVersion()
			require.Equal(t, tt.wantVersion, gotVersion)
		})
	}
}
