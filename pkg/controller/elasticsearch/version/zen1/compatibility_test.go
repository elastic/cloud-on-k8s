// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package zen1

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	es_sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
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

var testES = esv1.Elasticsearch{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "es1",
		Namespace: "default",
	},
}

func createMasterPodsWithVersion(ssetName, version string, replicas int32) []client.Object {
	pods := make([]client.Object, replicas)
	for i := int32(0); i < replicas; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      sset.PodName(ssetName, i),
				Namespace: "default",
				Labels: map[string]string{
					label.VersionLabelName:     version,
					label.ClusterNameLabelName: "es1",
				},
			},
		}
		label.NodeTypesMasterLabelName.Set(true, pod.Labels)
		pods[i] = pod
	}
	return pods
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
			if got := IsCompatibleWithZen1(context.Background(), tt.sset); got != tt.want {
				t.Errorf("IsCompatibleWithZen1() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAtLeastOneNodeCompatibleWithZen1(t *testing.T) {
	tests := []struct {
		name         string
		statefulSets es_sset.StatefulSetList
		client       k8s.Client
		want         bool
		wantErr      bool
	}{
		{
			name:         "no sset",
			statefulSets: nil,
			client:       k8s.NewFakeClient(),
			want:         false,
		},
		{
			name:         "none compatible",
			statefulSets: es_sset.StatefulSetList{createStatefulSetWithVersion("7.0.0"), createStatefulSetWithVersion("7.1.0")},
			client:       k8s.NewFakeClient(),
			want:         false,
		},
		{
			name:         "one compatible",
			statefulSets: es_sset.StatefulSetList{createStatefulSetWithVersion("6.8.0"), createStatefulSetWithVersion("7.1.0")},
			client:       k8s.NewFakeClient(),
			want:         true,
		},
		{
			name:         "all compatible",
			statefulSets: es_sset.StatefulSetList{createStatefulSetWithVersion("6.8.0"), createStatefulSetWithVersion("6.9.0")},
			client:       k8s.NewFakeClient(),
			want:         true,
		},
		{
			name:         "Version in StatefulSet spec in 7.2.0 but there're still some 6.8.0 in flight",
			statefulSets: es_sset.StatefulSetList{createStatefulSetWithVersion("7.2.0")},
			client:       k8s.NewFakeClient(createMasterPodsWithVersion("foo", "6.8.0", 5)...),
			want:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AtLeastOneNodeCompatibleWithZen1(context.Background(), tt.statefulSets, tt.client, testES)
			if (err != nil) != tt.wantErr {
				t.Errorf("runPredicates error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("AtLeastOneNodeCompatibleWithZen1() = %v, want %v", got, tt.want)
			}
		})
	}
}
