// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.
package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMapsEsAssociation_AssociationConfAnnotationName(t *testing.T) {
	k := ElasticMapsServer{}
	require.Equal(t, "association.k8s.elastic.co/es-conf", k.AssociationConfAnnotationName())
}
