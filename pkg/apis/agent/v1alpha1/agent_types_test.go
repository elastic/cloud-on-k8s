// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

func TestAgentESAssociation_AssociationConfAnnotationName(t *testing.T) {
	for _, tt := range []struct {
		name string
		ref  types.NamespacedName
		want string
	}{
		{
			name: "average length names",
			ref:  types.NamespacedName{Namespace: "namespace1", Name: "elasticsearch1"},
			want: "association.k8s.elastic.co/es-conf-XUQW524DHLS7APSORQVR76XZV6TVXVG623SITM7UUARH6",
		},
		{
			name: "max length namespace and name (63 and 36 respectively)",
			ref: types.NamespacedName{
				Namespace: "longnamespacelongnamespacelongnamespacelongnamespacelongnamespa",
				Name:      "elasticsearch1elasticsearch1elastics"},
			want: "association.k8s.elastic.co/es-conf-UT4MHW5EYE7CSF2BR4NBSAM4JHY3NEFSDPOCBXQPIZ6BK",
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
