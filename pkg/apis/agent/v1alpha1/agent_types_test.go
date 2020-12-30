// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

func TestAgentESAssociation_AssociationConfAnnotationName(t *testing.T) {
	aea := AgentESAssociation{
		ref: types.NamespacedName{Namespace: "namespace1", Name: "elasticsearch1"},
	}

	require.Equal(t, "association.k8s.elastic.co/es-conf-namespace1.elasticsearch1", aea.AssociationConfAnnotationName())
}
