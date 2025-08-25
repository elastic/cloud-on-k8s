// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// This mirrors what Maps has in maps_types_test.go
// EPR doesn't have associations like Maps does, so this is much simpler
func TestElasticPackageRegistry_ServiceAccountName(t *testing.T) {
	epr := ElasticPackageRegistry{}
	require.Equal(t, "", epr.ServiceAccountName())
}
