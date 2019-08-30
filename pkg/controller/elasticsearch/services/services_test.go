// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package services

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExternalServiceURL(t *testing.T) {
	type args struct {
		es v1alpha1.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "A service URL",
			args: args{es: v1alpha1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "an-es-name",
					Namespace: "default",
				},
			}},
			want: "https://an-es-name-es-http.default.svc:9200",
		},
		{
			name: "Another Service URL",
			args: args{es: v1alpha1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "another-es-name",
					Namespace: "default",
				},
			}},
			want: "https://another-es-name-es-http.default.svc:9200",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExternalServiceURL(tt.args.es)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestElasticsearchURL(t *testing.T) {
	type args struct {
		es   v1alpha1.Elasticsearch
		pods []corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "default: external service url",
			args: args{
				es: v1alpha1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-cluster",
						Namespace: "my-ns",
					},
				},
				pods: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								label.HTTPSchemeLabelName: "https",
							},
						},
					},
				},
			},
			want: "https://my-cluster-es-http.my-ns.svc:9200",
		},
		{
			name: "scheme change in progress: random pod address",
			args: args{
				es: v1alpha1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-cluster",
						Namespace: "my-ns",
					},
				},
				pods: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "my-ns",
							Name:      "my-sset-0",
							Labels: map[string]string{
								label.HTTPSchemeLabelName:      "http",
								label.StatefulSetNameLabelName: "my-sset",
							},
						},
					},
				},
			},
			want: "http://my-sset-0.my-sset.my-ns:9200",
		},
		{
			name: "unexpected: missing pod labels: fallback to service",
			args: args{
				es: v1alpha1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-cluster",
						Namespace: "my-ns",
					},
				},
				pods: []corev1.Pod{
					{},
				},
			},
			want: "https://my-cluster-es-http.my-ns.svc:9200",
		},
		{
			name: "unexpected: partially missing pod labels: fallback to service",
			args: args{
				es: v1alpha1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-cluster",
						Namespace: "my-ns",
					},
				},
				pods: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								label.HTTPSchemeLabelName: "http",
							},
						},
					},
				},
			},
			want: "https://my-cluster-es-http.my-ns.svc:9200",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ElasticsearchURL(tt.args.es, tt.args.pods); got != tt.want {
				t.Errorf("ElasticsearchURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
