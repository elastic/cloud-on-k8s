// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/network"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/compare"
)

func TestNewService(t *testing.T) {
	testCases := []struct {
		name     string
		services []logstashv1alpha1.LogstashService
		wantSvc  []func() corev1.Service
	}{
		{
			name: "single service",
			services: []logstashv1alpha1.LogstashService{{
				Name: "test",
				Service: commonv1.ServiceTemplate{
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Port: 9200},
						},
					},
				},
			},
			},
			wantSvc: []func() corev1.Service{
				func() corev1.Service {
					svc := mkService()
					svc.Name = "logstash-test-ls-test"
					svc.Spec.Ports[0].Port = 9200
					return svc
				},
			},
		},
		{
			name: "two services",
			services: []logstashv1alpha1.LogstashService{{
				Name: "test",
				Service: commonv1.ServiceTemplate{
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Port: 9200},
						},
					},
				},
			},
				{
					Name: "test2",
					Service: commonv1.ServiceTemplate{
						Spec: corev1.ServiceSpec{
							Ports: []corev1.ServicePort{
								{Port: 9300},
							},
						},
					},
				}},
			wantSvc: []func() corev1.Service{
				func() corev1.Service {
					svc := mkService()
					svc.Name = "logstash-test-ls-test"
					svc.Spec.Ports[0].Port = 9200
					return svc
				},
				func() corev1.Service {
					svc := mkService()
					svc.Name = "logstash-test-ls-test2"
					svc.Spec.Ports[0].Port = 9300
					return svc
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ls := mkLogstash(tc.services)
			for i := range tc.services {
				haveSvc := newService(tc.services[i], ls)
				compare.JSONEqual(t, tc.wantSvc[i](), haveSvc)
			}
		})
	}
}

func mkService() corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "logstash-test-ls-api",
			Namespace: "test",
			Labels: map[string]string{
				NameLabelName:          "logstash-test",
				commonv1.TypeLabelName: TypeLabelValue,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port: network.HTTPPort,
				},
			},
			Selector: map[string]string{
				NameLabelName:          "logstash-test",
				commonv1.TypeLabelName: TypeLabelValue,
			},
		},
	}
}

func mkLogstash(logstashServices []logstashv1alpha1.LogstashService) logstashv1alpha1.Logstash {
	return logstashv1alpha1.Logstash{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "logstash-test",
			Namespace: "test",
		},
		Spec: logstashv1alpha1.LogstashSpec{
			Services: logstashServices,
		},
	}
}