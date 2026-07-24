// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodelabels

import (
	"testing"

	"github.com/stretchr/testify/require"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
)

func TestValidateAnnotation(t *testing.T) {
	allowZone, err := NewExposedNodeLabels([]string{"topology.kubernetes.io/zone"})
	require.NoError(t, err)

	tests := []struct {
		name              string
		annotations       map[string]string
		exposedNodeLabels NodeLabels
		wantErrs          bool
	}{
		{
			name:              "annotation set, no exposed-node-labels configured",
			annotations:       map[string]string{commonv1.DownwardNodeLabelsAnnotation: "topology.kubernetes.io/zone"},
			exposedNodeLabels: nil,
			wantErrs:          true,
		},
		{
			name:              "annotation set, label matches the policy",
			annotations:       map[string]string{commonv1.DownwardNodeLabelsAnnotation: "topology.kubernetes.io/zone"},
			exposedNodeLabels: allowZone,
			wantErrs:          false,
		},
		{
			name:              "annotation set, label does not match the policy",
			annotations:       map[string]string{commonv1.DownwardNodeLabelsAnnotation: "topology.kubernetes.io/region"},
			exposedNodeLabels: allowZone,
			wantErrs:          true,
		},
		{
			name:              "no annotation set",
			annotations:       map[string]string{},
			exposedNodeLabels: allowZone,
			wantErrs:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateAnnotation(tt.annotations, tt.exposedNodeLabels)
			if tt.wantErrs {
				require.NotEmpty(t, errs)
			} else {
				require.Empty(t, errs)
			}
		})
	}
}
