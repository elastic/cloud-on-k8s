// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	appsv1 "k8s.io/api/apps/v1"
)

func TestGetUpdatePartition(t *testing.T) {
	tests := []struct {
		name        string
		statefulSet appsv1.StatefulSet
		want        int32
	}{
		{
			name:        "rolling update strategy not set: fallback to replicas",
			statefulSet: appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{Replicas: common.Int32(4)}},
			want:        4,
		},
		{
			name: "partition not set: fallback to replicas",
			statefulSet: appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{
				Replicas: common.Int32(4),
				UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
					RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{},
				}},
			},
			want: 4,
		},

		{
			name:        "rolling update partition & replicas not set: fallback to 0",
			statefulSet: appsv1.StatefulSet{},
			want:        0,
		},
		{
			name: "partition set: return it",
			statefulSet: appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{
				Replicas: common.Int32(4),
				UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
					RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{Partition: common.Int32(2)},
				},
			}},
			want: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetPartition(tt.statefulSet); got != tt.want {
				t.Errorf("GetPartition() = %v, want %v", got, tt.want)
			}
		})
	}
}
