// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestElasticsearchHealth_Less(t *testing.T) {

	tests := []struct {
		inputs []ElasticsearchHealth
		sorted bool
	}{
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchRedHealth,
				ElasticsearchYellowHealth,
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchRedHealth,
				ElasticsearchRedHealth,
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchRedHealth,
				ElasticsearchGreenHealth,
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchRedHealth,
				ElasticsearchYellowHealth,
				ElasticsearchGreenHealth,
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchYellowHealth,
				ElasticsearchGreenHealth,
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchGreenHealth,
				ElasticsearchYellowHealth,
			},
			sorted: false,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, sort.SliceIsSorted(tt.inputs, func(i, j int) bool {
			return tt.inputs[i].Less(tt.inputs[j])
		}), tt.sorted, fmt.Sprintf("%v", tt.inputs))
	}
}

func TestElasticsearchCluster_IsMarkedForDeletion(t *testing.T) {
	zeroTime := metav1.NewTime(time.Time{})
	currentTime := metav1.NewTime(time.Now())
	tests := []struct {
		name              string
		deletionTimestamp *metav1.Time
		want              bool
	}{
		{
			name:              "deletion timestamp nil",
			deletionTimestamp: nil,
			want:              false,
		},
		{
			name:              "deletion timestamp set to its zero value",
			deletionTimestamp: &zeroTime,
			want:              false,
		},
		{
			name:              "deletion timestamp set to any non-zero value",
			deletionTimestamp: &currentTime,
			want:              true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: tt.deletionTimestamp,
				},
			}
			require.Equal(t, tt.want, e.IsMarkedForDeletion())
		})
	}
}
