// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package services

import (
	"testing"

	commonv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/network"
	"github.com/elastic/cloud-on-k8s/pkg/utils/compare"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExternalServiceURL(t *testing.T) {
	type args struct {
		es v1beta1.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "A service URL",
			args: args{es: v1beta1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "an-es-name",
					Namespace: "default",
				},
			}},
			want: "https://an-es-name-es-http.default.svc:9200",
		},
		{
			name: "Another Service URL",
			args: args{es: v1beta1.Elasticsearch{
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
		es   v1beta1.Elasticsearch
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
				es: v1beta1.Elasticsearch{
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
				es: v1beta1.Elasticsearch{
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
				es: v1beta1.Elasticsearch{
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
				es: v1beta1.Elasticsearch{
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

func TestNewExternalService(t *testing.T) {
	testCases := []struct {
		name     string
		httpConf commonv1beta1.HTTPConfig
		wantSvc  func() corev1.Service
	}{
		{
			name: "no TLS",
			httpConf: commonv1beta1.HTTPConfig{
				TLS: commonv1beta1.TLSOptions{
					SelfSignedCertificate: &commonv1beta1.SelfSignedCertificate{
						Disabled: true,
					},
				},
			},
			wantSvc: mkService,
		},
		{
			name: "self-signed certificate",
			httpConf: commonv1beta1.HTTPConfig{
				TLS: commonv1beta1.TLSOptions{
					SelfSignedCertificate: &commonv1beta1.SelfSignedCertificate{
						SubjectAlternativeNames: []commonv1beta1.SubjectAlternativeName{
							{
								DNS: "elasticsearch-test.local",
							},
						},
					},
				},
			},
			wantSvc: func() corev1.Service {
				svc := mkService()
				svc.Spec.Ports[0].Name = "https"
				return svc
			},
		},
		{
			name: "user-provided certificate",
			httpConf: commonv1beta1.HTTPConfig{
				TLS: commonv1beta1.TLSOptions{
					Certificate: commonv1beta1.SecretRef{
						SecretName: "my-cert",
					},
				},
			},
			wantSvc: func() corev1.Service {
				svc := mkService()
				svc.Spec.Ports[0].Name = "https"
				return svc
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			es := mkElasticsearch(tc.httpConf)
			haveSvc := NewExternalService(es)
			compare.JSONEqual(t, tc.wantSvc(), haveSvc)
		})
	}
}

func mkService() corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "elasticsearch-test-es-http",
			Namespace: "test",
			Labels: map[string]string{
				label.ClusterNameLabelName: "elasticsearch-test",
				common.TypeLabelName:       label.Type,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Protocol: corev1.ProtocolTCP,
					Port:     network.HTTPPort,
				},
			},
			Selector: map[string]string{
				label.ClusterNameLabelName: "elasticsearch-test",
				common.TypeLabelName:       label.Type,
			},
		},
	}
}

func mkElasticsearch(httpConf commonv1beta1.HTTPConfig) v1beta1.Elasticsearch {
	return v1beta1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "elasticsearch-test",
			Namespace: "test",
		},
		Spec: v1beta1.ElasticsearchSpec{
			HTTP: httpConf,
		},
	}
}
