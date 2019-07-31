// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen2

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
)

func createStatefulSetWithESVersion(version string) appsv1.StatefulSet {
	return appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				label.VersionLabelName: version,
			},
		},
	}}}
}

func TestIsCompatibleForZen2(t *testing.T) {

	tests := []struct {
		name string
		sset appsv1.StatefulSet
		want bool
	}{
		{
			name: "version 6.8.0",
			sset: createStatefulSetWithESVersion("6.8.0"),
			want: false,
		},
		{
			name: "version 7.0.0",
			sset: createStatefulSetWithESVersion("7.0.0"),
			want: true,
		},
		{
			name: "no version",
			sset: createStatefulSetWithESVersion(""),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCompatibleForZen2(tt.sset); got != tt.want {
				t.Errorf("IsCompatibleForZen2() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAtLeastOneNodeCompatibleForZen2(t *testing.T) {
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
			statefulSets: sset.StatefulSetList{createStatefulSetWithESVersion("6.8.0"), createStatefulSetWithESVersion("6.8.1")},
			want:         false,
		},
		{
			name:         "one compatible",
			statefulSets: sset.StatefulSetList{createStatefulSetWithESVersion("6.8.0"), createStatefulSetWithESVersion("7.1.0")},
			want:         true,
		},
		{
			name:         "all compatible",
			statefulSets: sset.StatefulSetList{createStatefulSetWithESVersion("7.1.0"), createStatefulSetWithESVersion("7.2.0")},
			want:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AtLeastOneNodeCompatibleForZen2(tt.statefulSets); got != tt.want {
				t.Errorf("AtLeastOneNodeCompatibleForZen2() = %v, want %v", got, tt.want)
			}
		})
	}
}
