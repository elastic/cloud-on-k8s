// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen2

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
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

func TestIsCompatibleWithZen2(t *testing.T) {
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
			if got := IsCompatibleWithZen2(tt.sset); got != tt.want {
				t.Errorf("IsCompatibleWithZen2() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllMastersCompatibleWithZen2(t *testing.T) {
	es := v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "cluster",
		},
	}
	tests := []struct {
		name string
		pods []runtime.Object
		want bool
	}{
		{
			name: "only v7 master nodes",
			pods: []runtime.Object{
				nodespec.TestPod{Namespace: es.Namespace, Name: "node0", ClusterName: es.Name, Version: "7.2.0", Master: true}.BuildPtr(),
				nodespec.TestPod{Namespace: es.Namespace, Name: "node1", ClusterName: es.Name, Version: "7.2.0", Master: true}.BuildPtr(),
				nodespec.TestPod{Namespace: es.Namespace, Name: "node2", ClusterName: es.Name, Version: "7.2.0", Data: true}.BuildPtr(),
			},
			want: true,
		},
		{
			name: "only v6 master nodes (with v7 data nodes)",
			pods: []runtime.Object{
				nodespec.TestPod{Namespace: es.Namespace, Name: "node0", ClusterName: es.Name, Version: "6.8.0", Master: true}.BuildPtr(),
				nodespec.TestPod{Namespace: es.Namespace, Name: "node1", ClusterName: es.Name, Version: "6.8.0", Master: true}.BuildPtr(),
				nodespec.TestPod{Namespace: es.Namespace, Name: "node2", ClusterName: es.Name, Version: "7.2.0", Data: true}.BuildPtr(),
			},
			want: false,
		},
		{
			name: "mixed v6/v7 masters",
			pods: []runtime.Object{
				nodespec.TestPod{Namespace: es.Namespace, Name: "node0", ClusterName: es.Name, Version: "7.2.0", Master: true}.BuildPtr(),
				nodespec.TestPod{Namespace: es.Namespace, Name: "node1", ClusterName: es.Name, Version: "6.8.0", Master: true}.BuildPtr(),
			},
			want: false,
		},
		{
			name: "no pods",
			pods: []runtime.Object{},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AllMastersCompatibleWithZen2(k8s.WrapClient(fake.NewFakeClient(tt.pods...)), es)
			require.NoError(t, err)
			if got != tt.want {
				t.Errorf("AllMastersCompatibleWithZen2() got = %v, want %v", got, tt.want)
			}
		})
	}
}
