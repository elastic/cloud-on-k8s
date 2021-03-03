// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.
package v1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApmEsAssociation_AssociationConfAnnotationName(t *testing.T) {
	k := Kibana{}
	require.Equal(t, "association.k8s.elastic.co/es-conf", k.AssociationConfAnnotationName())
}
