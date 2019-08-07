// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen1

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
)

func createStatefulSetWithVersion(version string) appsv1.StatefulSet {
	return appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				label.VersionLabelName: version,
			},
		},
	}}}
}

func TestIsCompatibleWithZen1(t *testing.T) {

	tests := []struct {
		name string
		sset appsv1.StatefulSet
		want bool
	}{
		{
			name: "version 6.8.0",
			sset: createStatefulSetWithVersion("6.8.0"),
			want: true,
		},
		{
			name: "version 7.0.0",
			sset: createStatefulSetWithVersion("7.0.0"),
			want: false,
		},
		{
			name: "no version",
			sset: createStatefulSetWithVersion(""),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCompatibleWithZen1(tt.sset); got != tt.want {
				t.Errorf("IsCompatibleWithZen1() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAtLeastOneNodeCompatibleWithZen1(t *testing.T) {
	tests := []struct {
		name         string
		statefulSets sset.StatefulSetList
		want         bool
	}{
		{
			name:         "no sset",
			statefulSets: nil,
			want:         false,
		},
		{
			name:         "none compatible",
			statefulSets: sset.StatefulSetList{createStatefulSetWithVersion("7.0.0"), createStatefulSetWithVersion("7.1.0")},
			want:         false,
		},
		{
			name:         "one compatible",
			statefulSets: sset.StatefulSetList{createStatefulSetWithVersion("6.8.0"), createStatefulSetWithVersion("7.1.0")},
			want:         true,
		},
		{
			name:         "all compatible",
			statefulSets: sset.StatefulSetList{createStatefulSetWithVersion("6.8.0"), createStatefulSetWithVersion("6.9.0")},
			want:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AtLeastOneNodeCompatibleWithZen1(tt.statefulSets); got != tt.want {
				t.Errorf("AtLeastOneNodeCompatibleWithZen1() = %v, want %v", got, tt.want)
			}
		})
	}
}
