// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
)

func TestRenderSuspendConfiguration(t *testing.T) {
	tests := []struct {
		name string
		es   esv1.Elasticsearch
		want string
	}{
		{
			name: "no annotation",
			es:   esv1.Elasticsearch{},
			want: "",
		},
		{
			name: "single value",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						esv1.SuspendAnnotation: "pod-1",
					},
				},
			},
			want: "pod-1",
		},
		{
			name: "multi value",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						esv1.SuspendAnnotation: "pod-1,pod-2",
					},
				},
			},
			want: `pod-1
pod-2`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RenderSuspendConfiguration(tt.es); got != tt.want {
				t.Errorf("RenderSuspendConfiguration() = %v, want %v", got, tt.want)
			}
		})
	}
}
