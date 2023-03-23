// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/network"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/compare"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
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
	trueVal := true
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "logstash-test-ls-api",
			Namespace: "test",
			Labels: map[string]string{
				NameLabelName:          "logstash-test",
				commonv1.TypeLabelName: TypeLabelValue,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "logstash.k8s.elastic.co/v1alpha1",
					Kind:               "Logstash",
					Name:               "logstash-test",
					Controller:         &trueVal,
					BlockOwnerDeletion: &trueVal,
				},
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

func TestReconcileServices(t *testing.T) {
	trueVal := true
	testCases := []struct {
		name     string
		logstash logstashv1alpha1.Logstash
		wantSvc  []corev1.Service
	}{
		{
			name: "default service",
			logstash: 	logstashv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "logstash",
					Namespace: "test",
				},
			},
			wantSvc: [] corev1.Service{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "logstash-ls-api",
					Namespace: "test",
					Labels: map[string]string{
						"common.k8s.elastic.co/type":   "logstash",
						"logstash.k8s.elastic.co/name": "logstash",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "logstash.k8s.elastic.co/v1alpha1",
							Kind:               "Logstash",
							Name:               "logstash",
							Controller:         &trueVal,
							BlockOwnerDeletion: &trueVal,
						},
					},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						"common.k8s.elastic.co/type":   "logstash",
						"logstash.k8s.elastic.co/name": "logstash",
					},
					ClusterIP: "None",
					Ports: []corev1.ServicePort{
						{Name: "api", Protocol: "TCP", Port: 9600},
					},
				},
			},},
		},
		{
			name: "Changed port on default service",
			logstash: 	logstashv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "logstash",
					Namespace: "test",
				},
				Spec: logstashv1alpha1.LogstashSpec{
					Services: []logstashv1alpha1.LogstashService{{
						Name: "api",
						Service: commonv1.ServiceTemplate{
							Spec: corev1.ServiceSpec{
								Ports: []corev1.ServicePort{
									{Name: "api", Protocol: "TCP", Port: 9200},
								},
							},
						},
					},},
				},
			},
			wantSvc: [] corev1.Service{{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "logstash-ls-api",
					Namespace: "test",
					Labels: map[string]string{
						"common.k8s.elastic.co/type":   "logstash",
						"logstash.k8s.elastic.co/name": "logstash",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "logstash.k8s.elastic.co/v1alpha1",
							Kind:               "Logstash",
							Name:               "logstash",
							Controller:         &trueVal,
							BlockOwnerDeletion: &trueVal,
						},
					},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						"common.k8s.elastic.co/type":   "logstash",
						"logstash.k8s.elastic.co/name": "logstash",
					},
					ClusterIP: "None",
					Ports: []corev1.ServicePort{
						{Name: "api", Protocol: "TCP", Port: 9200},
					},
				},
			},},
		},
		{
			name: "Default service plus one",
			logstash: 	logstashv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "logstash",
					Namespace: "test",
				},
				Spec: logstashv1alpha1.LogstashSpec{
					Services: []logstashv1alpha1.LogstashService{{
						Name: "test",
						Service: commonv1.ServiceTemplate{
							Spec: corev1.ServiceSpec{
								Ports: []corev1.ServicePort{
									{Protocol: "TCP", Port: 9500},
								},
							},
						},
					},},
				},
			},
			wantSvc: [] corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "logstash-ls-test",
						Namespace: "test",
						Labels: map[string]string{
							"common.k8s.elastic.co/type":   "logstash",
							"logstash.k8s.elastic.co/name": "logstash",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "logstash.k8s.elastic.co/v1alpha1",
								Kind:               "Logstash",
								Name:               "logstash",
								Controller:         &trueVal,
								BlockOwnerDeletion: &trueVal,
							},
						},
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"common.k8s.elastic.co/type":   "logstash",
							"logstash.k8s.elastic.co/name": "logstash",
						},
						Ports: []corev1.ServicePort{
							{Protocol: "TCP", Port: 9500},
						},
					},
				},
				{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "logstash-ls-api",
					Namespace: "test",
					Labels: map[string]string{
						"common.k8s.elastic.co/type":   "logstash",
						"logstash.k8s.elastic.co/name": "logstash",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "logstash.k8s.elastic.co/v1alpha1",
							Kind:               "Logstash",
							Name:               "logstash",
							Controller:         &trueVal,
							BlockOwnerDeletion: &trueVal,
						},
					},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						"common.k8s.elastic.co/type":   "logstash",
						"logstash.k8s.elastic.co/name": "logstash",
					},
					ClusterIP: "None",
					Ports: []corev1.ServicePort{
						{Name: "api", Protocol: "TCP", Port: 9600},
					},
				},
			},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defaultSvc := defaultServiceFromOwner(&tc.logstash)
			client := k8s.NewFakeClient(&tc.logstash, defaultSvc)
			params := Params{
				Context:        context.Background(),
				Client:         client,
				Logstash:       tc.logstash,
			}
			haveSvc, err := reconcileServices(params)
			require.NoError(t, err)
			require.Equal(t, len(tc.wantSvc), len(haveSvc))

			for i := range tc.wantSvc {
				comparison.AssertEqual(t, &tc.wantSvc[i], &haveSvc[i])
			}
		})
	}
}



func defaultServiceFromOwner(owner *logstashv1alpha1.Logstash) *corev1.Service {
	trueVal := true
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "logstash-ls-api",
			Namespace:   "test",
			Labels: map[string]string{
				"common.k8s.elastic.co/type":   "logstash",
				"logstash.k8s.elastic.co/name": "logstash",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "logstash.k8s.elastic.co/v1alpha1",
					Kind:               "Logstash",
					Name:               owner.Name,
					Controller:         &trueVal,
					BlockOwnerDeletion: &trueVal,
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
					"common.k8s.elastic.co/type":   "logstash",
					"logstash.k8s.elastic.co/name": "logstash",
			},
			ClusterIP:  "None",
			Ports: []corev1.ServicePort{
				{Name: "api", Protocol: "TCP", Port: 9600},
			},
		},
	}
}
