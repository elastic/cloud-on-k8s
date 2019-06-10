// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_newPVCFromTemplate(t *testing.T) {
	type args struct {
		claimTemplate corev1.PersistentVolumeClaim
		pod           *corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want *corev1.PersistentVolumeClaim
	}{
		{
			name: "Create a simple PVC from a template and a pod",
			args: args{
				claimTemplate: corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: volume.ElasticsearchDataVolumeName,
					},
				},
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "elasticsearch-sample-es-6bw9qkw77k",
						Labels: map[string]string{
							"l1": "v1",
							"l2": "v2",
						},
					},
				},
			},
			want: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "elasticsearch-sample-es-6bw9qkw77k-" + volume.ElasticsearchDataVolumeName,
					Labels: map[string]string{
						"l1":                   "v1",
						"l2":                   "v2",
						label.PodNameLabelName: "elasticsearch-sample-es-6bw9qkw77k",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newPVCFromTemplate(tt.args.claimTemplate, tt.args.pod); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newPVCFromTemplate() = %v, want %v", got, tt.want)
			}
		})
	}
}
