// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodespec

import (
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
)

func createSset(name string, nodeCount int32, master bool, data bool) appsv1.StatefulSet {
	labels := map[string]string{}
	label.NodeTypesMasterLabelName.Set(master, labels)
	label.NodeTypesDataLabelName.Set(data, labels)
	return appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &nodeCount,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
			},
		},
	}
}

func TestResourcesList_MasterNodesNames(t *testing.T) {
	tests := []struct {
		name string
		l    ResourcesList
		want []string
	}{
		{
			name: "no nodes",
			l:    nil,
			want: nil,
		},
		{
			name: "3 master-only nodes, 3 master-data nodes, 3 data nodes",
			l: ResourcesList{
				{StatefulSet: createSset("master-only", 3, true, false)},
				{StatefulSet: createSset("master-data", 3, true, true)},
				{StatefulSet: createSset("data-only", 3, false, true)},
			},
			want: []string{
				"master-only-0", "master-only-1", "master-only-2",
				"master-data-0", "master-data-1", "master-data-2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.l.MasterNodesNames(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ResourcesList.MasterNodesNames() = %v, want %v", got, tt.want)
			}
		})
	}
}
