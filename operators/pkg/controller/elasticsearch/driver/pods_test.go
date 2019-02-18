// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"reflect"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
					ObjectMeta: v1.ObjectMeta{
						Name: "data",
					},
				},
				pod: &corev1.Pod{
					ObjectMeta: v1.ObjectMeta{
						Name: "elasticsearch-sample-es-6bw9qkw77k",
						Labels: map[string]string{
							"l1": "v1",
							"l2": "v2",
						},
					},
				},
			},
			want: &corev1.PersistentVolumeClaim{
				ObjectMeta: v1.ObjectMeta{
					Name: "elasticsearch-sample-es-6bw9qkw77k-data",
					Labels: map[string]string{
						"l1":                    "v1",
						"l2":                    "v2",
						label.NodeNameLabelName: "elasticsearch-sample-es-6bw9qkw77k",
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
