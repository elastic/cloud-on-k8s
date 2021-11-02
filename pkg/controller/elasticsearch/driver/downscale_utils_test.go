// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
)

func Test_ssetDownscale_leavingNodeNames(t *testing.T) {
	tests := []struct {
		name            string
		statefulSet     appsv1.StatefulSet
		initialReplicas int32
		targetReplicas  int32
		want            []string
	}{
		{
			name:            "no replicas",
			statefulSet:     ssetMaster3Replicas,
			initialReplicas: 0,
			targetReplicas:  0,
			want:            nil,
		},
		{
			name:            "going from 2 to 0 replicas",
			statefulSet:     ssetMaster3Replicas,
			initialReplicas: 2,
			targetReplicas:  0,
			want:            []string{"ssetMaster3Replicas-1", "ssetMaster3Replicas-0"},
		},
		{
			name:            "going from 2 to 1 replicas",
			statefulSet:     ssetMaster3Replicas,
			initialReplicas: 2,
			targetReplicas:  1,
			want:            []string{"ssetMaster3Replicas-1"},
		},
		{
			name:            "going from 5 to 2 replicas",
			statefulSet:     ssetMaster3Replicas,
			initialReplicas: 5,
			targetReplicas:  2,
			want:            []string{"ssetMaster3Replicas-4", "ssetMaster3Replicas-3", "ssetMaster3Replicas-2"},
		},
		{
			name:            "no replicas change",
			statefulSet:     ssetMaster3Replicas,
			initialReplicas: 2,
			targetReplicas:  2,
			want:            nil,
		},
		{
			name:            "upscale",
			statefulSet:     ssetMaster3Replicas,
			initialReplicas: 2,
			targetReplicas:  3,
			want:            nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := ssetDownscale{
				statefulSet:     tt.statefulSet,
				initialReplicas: tt.initialReplicas,
				targetReplicas:  tt.targetReplicas,
			}
			if got := d.leavingNodeNames(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("leavingNodeNames() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_leavingNodeNames(t *testing.T) {
	tests := []struct {
		name       string
		downscales []ssetDownscale
		want       []string
	}{
		{
			name:       "no downscales",
			downscales: nil,
			want:       []string{},
		},
		{
			name: "2 downscales",
			downscales: []ssetDownscale{
				{
					statefulSet:     ssetMaster3Replicas,
					initialReplicas: 2,
					targetReplicas:  1,
				},
				{
					statefulSet:     ssetData4Replicas,
					initialReplicas: 4,
					targetReplicas:  3,
				},
			},
			want: []string{"ssetMaster3Replicas-1", "ssetData4Replicas-3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := leavingNodeNames(tt.downscales); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("leavingNodeNames() = %v, want %v", got, tt.want)
			}
		})
	}
}
