// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
)

func TestAgentESAssociation_AssociationConfAnnotationName(t *testing.T) {
	for _, tt := range []struct {
		name string
		ref  commonv1.ElasticsearchSelector
		want string
	}{
		{
			name: "average length names",
			ref: commonv1.ElasticsearchSelector{
				ObjectSelector: commonv1.ObjectSelector{Namespace: "namespace1", Name: "elasticsearch1"},
			},
			want: "association.k8s.elastic.co/es-conf-2150608354",
		},
		{
			name: "max length namespace and name (63 and 36 respectively)",
			ref: commonv1.ElasticsearchSelector{
				ObjectSelector: commonv1.ObjectSelector{
					Namespace: "longnamespacelongnamespacelongnamespacelongnamespacelongnamespa",
					Name:      "elasticsearch1elasticsearch1elastics",
				},
			},
			want: "association.k8s.elastic.co/es-conf-3419573237",
		},
		{
			name: "secret name gives a different hash",
			ref: commonv1.ElasticsearchSelector{
				ObjectSelector: commonv1.ObjectSelector{Namespace: "namespace1", SecretName: "elasticsearch1"},
			},
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

func TestAgent_GetAssociations(t *testing.T) {
	defaultNamespace := "default"

	for _, tt := range []struct {
		name               string
		spec               AgentSpec
		assertAssociations func(*testing.T, []commonv1.Association)
	}{
		{
			name: "custom role is preserved",
			spec: AgentSpec{
				ElasticsearchRefs: []Output{
					{
						ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "es1", Namespace: "ns1"}},
						ElasticsearchRole:     "custom_role",
					},
				},
			},
			assertAssociations: func(t *testing.T, ca []commonv1.Association) {
				t.Helper()
				require.Len(t, ca, 1)
				a, ok := ca[0].(commonv1.AssociationWithUserRoleOverride)
				require.True(t, ok)
				require.Equal(t, "custom_role", a.UserRoleOverride())
			},
		},
		{
			name: "empty role when not set",
			spec: AgentSpec{
				ElasticsearchRefs: []Output{
					{
						ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "es1", Namespace: "ns1"}},
					},
				},
			},
			assertAssociations: func(t *testing.T, ca []commonv1.Association) {
				t.Helper()
				require.Len(t, ca, 1)
				a, ok := ca[0].(commonv1.AssociationWithUserRoleOverride)
				require.True(t, ok)
				require.Equal(t, "", a.UserRoleOverride())
			},
		},
		{
			name: "role preserved per-ref across multiple refs with default namespace",
			spec: AgentSpec{
				ElasticsearchRefs: []Output{
					{
						ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "es1"}},
						ElasticsearchRole:     "custom_role",
					},
					{
						ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "es2"}},
					},
				},
			},
			assertAssociations: func(t *testing.T, ca []commonv1.Association) {
				t.Helper()
				require.Len(t, ca, 2)
				a0, ok := ca[0].(commonv1.AssociationWithUserRoleOverride)
				require.True(t, ok)
				require.Equal(t, "custom_role", a0.UserRoleOverride())
				require.Equal(t, defaultNamespace, a0.AssociationRef().GetNamespace())
				a1, ok := ca[1].(commonv1.AssociationWithUserRoleOverride)
				require.True(t, ok)
				require.Equal(t, "", a1.UserRoleOverride())
				require.Equal(t, defaultNamespace, a1.AssociationRef().GetNamespace())
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := Agent{ObjectMeta: v1.ObjectMeta{Namespace: defaultNamespace}, Spec: tt.spec}
			associations := a.GetAssociations()
			tt.assertAssociations(t, associations)
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
