// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestElasticsearchHealth_Less(t *testing.T) {
	tests := []struct {
		inputs []ElasticsearchHealth
		sorted bool
	}{
		{
			inputs: []ElasticsearchHealth{
				"",
				ElasticsearchYellowHealth,
				"",
			},
			sorted: true,
		},
		{
			inputs: []ElasticsearchHealth{
				ElasticsearchUnknownHealth,
				ElasticsearchYellowHealth,
				ElasticsearchUnknownHealth,
			},
			sorted: true,
		},
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
func Test_GetMaxSurgeOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		fromSpec *int32
		want     *int32
	}{
		{
			name:     "negative in spec results in unbounded",
			fromSpec: ptr.To[int32](-1),
			want:     nil,
		},
		{
			name:     "nil in spec results in default, generic",
			fromSpec: nil,
			want:     DefaultChangeBudget.MaxSurge,
		},
		{
			name:     "nil in spec results in default, currently nil",
			fromSpec: nil,
			want:     nil,
		},
		{
			name:     "0 in spec results in 0",
			fromSpec: ptr.To[int32](0),
			want:     ptr.To[int32](0),
		},
		{
			name:     "1 in spec results in 1",
			fromSpec: ptr.To[int32](1),
			want:     ptr.To[int32](1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChangeBudget{MaxSurge: tt.fromSpec}.GetMaxSurgeOrDefault()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetMaxSurgeOrDefault() want = %v, got = %v", tt.want, got)
			}
		})
	}
}

func Test_GetMaxUnavailableOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		fromSpec *int32
		want     *int32
	}{
		{
			name:     "negative in spec results in unbounded",
			fromSpec: ptr.To[int32](-1),
			want:     nil,
		},
		{
			name:     "nil in spec results in default, generic",
			fromSpec: nil,
			want:     DefaultChangeBudget.MaxUnavailable,
		},
		{
			name:     "nil in spec results in default, currently 1",
			fromSpec: nil,
			want:     ptr.To[int32](1),
		},
		{
			name:     "0 in spec results in 0",
			fromSpec: ptr.To[int32](0),
			want:     ptr.To[int32](0),
		},
		{
			name:     "1 in spec results in 1",
			fromSpec: ptr.To[int32](1),
			want:     ptr.To[int32](1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChangeBudget{MaxUnavailable: tt.fromSpec}.GetMaxUnavailableOrDefault()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetMaxUnavailableOrDefault() want = %v, got = %v", tt.want, got)
			}
		})
	}
}
