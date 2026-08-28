// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeClaim(name, storageSize string) corev1.PersistentVolumeClaim {
	pvc := corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if storageSize != "" {
		pvc.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceStorage: resource.MustParse(storageSize),
		}
	}
	return pvc
}

func TestStorageOverrideClaim(t *testing.T) {
	tests := []struct {
		name      string
		claims    []corev1.PersistentVolumeClaim
		wantIdx   int
		wantFound bool
	}{
		{
			name:      "nil claims: not found",
			claims:    nil,
			wantIdx:   -1,
			wantFound: false,
		},
		{
			name:      "named data claim: returned first",
			claims:    []corev1.PersistentVolumeClaim{makeClaim(ElasticsearchDataVolumeName, "1Gi")},
			wantIdx:   0,
			wantFound: true,
		},
		{
			name: "named data claim among several: correct index returned",
			claims: []corev1.PersistentVolumeClaim{
				makeClaim("other", "2Gi"),
				makeClaim(ElasticsearchDataVolumeName, "1Gi"),
			},
			wantIdx:   1,
			wantFound: true,
		},
		{
			name:      "sole non-default-named claim: fallback returns it",
			claims:    []corev1.PersistentVolumeClaim{makeClaim("custom", "1Gi")},
			wantIdx:   0,
			wantFound: true,
		},
		{
			name: "multiple non-default-named claims: not found",
			claims: []corev1.PersistentVolumeClaim{
				makeClaim("custom-a", "1Gi"),
				makeClaim("custom-b", "2Gi"),
			},
			wantIdx:   -1,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIdx, gotFound := StorageOverrideClaim(tt.claims)
			assert.Equal(t, tt.wantIdx, gotIdx)
			assert.Equal(t, tt.wantFound, gotFound)
		})
	}
}

func TestApplyStorageOverride(t *testing.T) {
	size10Gi := resource.MustParse("10Gi")

	tests := []struct {
		name        string
		claims      []corev1.PersistentVolumeClaim
		storage     *resource.Quantity
		wantStorage map[string]string // claim name → expected storage string; "" means Requests is nil/absent
		wantSame    bool              // true when the returned slice must be the exact same input (no copy)
	}{
		{
			name:     "nil storage returns input untouched",
			claims:   []corev1.PersistentVolumeClaim{makeClaim(ElasticsearchDataVolumeName, "1Gi")},
			storage:  nil,
			wantSame: true,
		},
		{
			name:     "nil claims returns nil",
			claims:   nil,
			storage:  &size10Gi,
			wantSame: true,
		},
		{
			name:        "named data claim is updated",
			claims:      []corev1.PersistentVolumeClaim{makeClaim(ElasticsearchDataVolumeName, "1Gi")},
			storage:     &size10Gi,
			wantStorage: map[string]string{ElasticsearchDataVolumeName: "10Gi"},
		},
		{
			name: "named data claim updated; other claims untouched",
			claims: []corev1.PersistentVolumeClaim{
				makeClaim("other", "2Gi"),
				makeClaim(ElasticsearchDataVolumeName, "1Gi"),
			},
			storage: &size10Gi,
			wantStorage: map[string]string{
				"other":                     "2Gi",
				ElasticsearchDataVolumeName: "10Gi",
			},
		},
		{
			name:        "sole non-default-named claim is updated",
			claims:      []corev1.PersistentVolumeClaim{makeClaim("custom-data", "1Gi")},
			storage:     &size10Gi,
			wantStorage: map[string]string{"custom-data": "10Gi"},
		},
		{
			name: "multiple non-default-named claims: returns untouched",
			claims: []corev1.PersistentVolumeClaim{
				makeClaim("custom-a", "1Gi"),
				makeClaim("custom-b", "2Gi"),
			},
			storage:  &size10Gi,
			wantSame: true,
		},
		{
			name:        "claim with nil Requests gets storage set",
			claims:      []corev1.PersistentVolumeClaim{makeClaim(ElasticsearchDataVolumeName, "")},
			storage:     &size10Gi,
			wantStorage: map[string]string{ElasticsearchDataVolumeName: "10Gi"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyStorageOverride(tt.claims, tt.storage)

			if tt.wantSame {
				if len(tt.claims) > 0 {
					require.Len(t, got, len(tt.claims))
					assert.True(t, &got[0] == &tt.claims[0], "expected same backing array, not a copy")
				} else {
					assert.Nil(t, got)
				}
				return
			}

			require.Len(t, got, len(tt.wantStorage))
			// Verify tt.claims were deep copied
			assert.True(t, &got[0] != &tt.claims[0], "expected a new backing array, got the same slice")
			for _, claim := range got {
				wantSize, ok := tt.wantStorage[claim.Name]
				require.True(t, ok, "unexpected claim %q in output", claim.Name)
				gotSize := claim.Spec.Resources.Requests[corev1.ResourceStorage]
				assert.Equal(t, wantSize, gotSize.String())
			}

			// output must be a deep copy: mutating it must not affect the input
			for i := range got {
				if _, hasStorage := got[i].Spec.Resources.Requests[corev1.ResourceStorage]; hasStorage {
					mutated := resource.MustParse("99Gi")
					got[i].Spec.Resources.Requests[corev1.ResourceStorage] = mutated
				}
			}
			for _, orig := range tt.claims {
				if origSize, ok := orig.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
					assert.NotEqual(t, "99Gi", origSize.String(), "deep-copy must isolate output from input for claim %q", orig.Name)
				}
			}
		})
	}
}
