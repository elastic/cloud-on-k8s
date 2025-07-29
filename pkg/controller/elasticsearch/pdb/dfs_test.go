// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pdb

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"

	ssetfixtures "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
)

func TestGroupBySharedRoles(t *testing.T) {
	tests := []struct {
		name         string
		statefulSets sset.StatefulSetList
		want         [][]appsv1.StatefulSet
	}{
		{
			name:         "empty statefulsets",
			statefulSets: sset.StatefulSetList{},
			want:         [][]appsv1.StatefulSet{},
		},
		{
			name: "single statefulset with no roles",
			statefulSets: sset.StatefulSetList{
				ssetfixtures.TestSset{Name: "coordinating"}.Build(),
			},
			want: [][]appsv1.StatefulSet{
				{
					ssetfixtures.TestSset{Name: "coordinating"}.Build(),
				},
			},
		},
		{
			name: "all statefulsets with different roles",
			statefulSets: sset.StatefulSetList{
				ssetfixtures.TestSset{Name: "master", Master: true}.Build(),
				ssetfixtures.TestSset{Name: "ingest", Ingest: true}.Build(),
			},
			want: [][]appsv1.StatefulSet{
				{
					ssetfixtures.TestSset{Name: "master", Master: true}.Build(),
				},
				{
					ssetfixtures.TestSset{Name: "ingest", Ingest: true}.Build(),
				},
			},
		},
		{
			name: "statefulsets with shared roles are grouped properly",
			statefulSets: sset.StatefulSetList{
				ssetfixtures.TestSset{Name: "master", Master: true, Data: true}.Build(),
				ssetfixtures.TestSset{Name: "data", Data: true}.Build(),
				ssetfixtures.TestSset{Name: "ingest", Ingest: true}.Build(),
			},
			want: [][]appsv1.StatefulSet{
				{
					ssetfixtures.TestSset{Name: "master", Master: true, Data: true}.Build(),
					ssetfixtures.TestSset{Name: "data", Data: true}.Build(),
				},
				{
					ssetfixtures.TestSset{Name: "ingest", Ingest: true}.Build(),
				},
			},
		},
		{
			name: "statefulsets with multiple shared roles in multiple groups, and data* roles are grouped properly",
			statefulSets: sset.StatefulSetList{
				ssetfixtures.TestSset{Name: "master", Master: true, Data: true}.Build(),
				ssetfixtures.TestSset{Name: "data", Data: true}.Build(),
				ssetfixtures.TestSset{Name: "data_hot", DataHot: true}.Build(),
				ssetfixtures.TestSset{Name: "data_warm", DataWarm: true}.Build(),
				ssetfixtures.TestSset{Name: "data_cold", DataCold: true}.Build(),
				ssetfixtures.TestSset{Name: "ingest", Ingest: true, ML: true}.Build(),
				ssetfixtures.TestSset{Name: "ml", ML: true}.Build(),
			},
			want: [][]appsv1.StatefulSet{
				{
					ssetfixtures.TestSset{Name: "master", Master: true, Data: true}.Build(),
					ssetfixtures.TestSset{Name: "data", Data: true}.Build(),
					ssetfixtures.TestSset{Name: "data_hot", DataHot: true}.Build(),
					ssetfixtures.TestSset{Name: "data_warm", DataWarm: true}.Build(),
					ssetfixtures.TestSset{Name: "data_cold", DataCold: true}.Build(),
				},
				{
					ssetfixtures.TestSset{Name: "ingest", Ingest: true, ML: true}.Build(),
					ssetfixtures.TestSset{Name: "ml", ML: true}.Build(),
				},
			},
		},
		{
			name: "coordinating nodes (no roles) in separate group",
			statefulSets: sset.StatefulSetList{
				ssetfixtures.TestSset{Name: "data", Data: true}.Build(),
				ssetfixtures.TestSset{Name: "coordinating1"}.Build(),
				ssetfixtures.TestSset{Name: "coordinating2"}.Build(),
			},
			want: [][]appsv1.StatefulSet{
				{
					ssetfixtures.TestSset{Name: "data", Data: true}.Build(),
				},
				{
					ssetfixtures.TestSset{Name: "coordinating1"}.Build(),
					ssetfixtures.TestSset{Name: "coordinating2"}.Build(),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := groupBySharedRoles(tt.statefulSets)
			sortStatefulSetGroups(tt.want)
			sortStatefulSetGroups(got)
			assert.Equal(t, len(tt.want), len(got), "Expected %d groups, got %d", len(tt.want), len(got))

			for i := 0; i < len(tt.want); i++ {
				if i >= len(got) {
					t.Errorf("Missing group at index %d", i)
					continue
				}

				assert.Equal(t, len(tt.want[i]), len(got[i]), "Group %d has wrong size", i)

				// Check if all StatefulSets in the group match
				for j := 0; j < len(tt.want[i]); j++ {
					if j >= len(got[i]) {
						t.Errorf("Missing StatefulSet at index %d in group %d", j, i)
						continue
					}

					assert.Equal(t, tt.want[i][j].Name, got[i][j].Name, "StatefulSet names do not match in group %d", i)
					assert.Equal(t, tt.want[i][j].Spec.Template.Labels, got[i][j].Spec.Template.Labels, "StatefulSet labels do not match in group %d", i)
				}
			}
		})
	}
}

// sortStatefulSetGroups sorts the groups and StatefulSets within groups by name
// for consistent comparison in tests
func sortStatefulSetGroups(groups [][]appsv1.StatefulSet) {
	// First sort each group internally by StatefulSet names
	for i := range groups {
		slices.SortFunc(groups[i], func(a, b appsv1.StatefulSet) int {
			return strings.Compare(a.Name, b.Name)
		})
	}

	// Then sort the groups by the name of the first StatefulSet in each group
	slices.SortFunc(groups, func(a, b []appsv1.StatefulSet) int {
		// Empty groups come last
		if len(a) == 0 {
			return 1
		}
		if len(b) == 0 {
			return -1
		}
		// Compare first StatefulSet names
		return strings.Compare(a[0].Name, b[0].Name)
	})
}

// TestNormalizeRole tests the normalizeRole function
func TestNormalizeRole(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		expected string
	}{
		{
			name:     "data role should remain the same",
			role:     "data",
			expected: "data",
		},
		{
			name:     "data_hot role should be normalized to data",
			role:     "data_hot",
			expected: "data",
		},
		{
			name:     "data_frozen role should remain the same",
			role:     "data_frozen",
			expected: "data_frozen",
		},
		{
			name:     "other roles should remain the same",
			role:     "master",
			expected: "master",
		},
		{
			name:     "empty role should remain empty",
			role:     "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeRole(tt.role)
			assert.Equal(t, tt.expected, got)
		})
	}
}
