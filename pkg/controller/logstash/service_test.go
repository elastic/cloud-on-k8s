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
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestReconcileServices(t *testing.T) {
	trueVal := true
	testCases := []struct {
		name     string
		logstash logstashv1alpha1.Logstash
		wantSvc  []corev1.Service
	}{
		{
			name: "default service",
			logstash: logstashv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "logstash",
					Namespace: "test",
				},
			},
			wantSvc: []corev1.Service{{
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
			}},
		},
		{
			name: "Changed port on default service",
			logstash: logstashv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "logstash",
					Namespace: "test",
				},
				Spec: logstashv1alpha1.LogstashSpec{
					Services: []logstashv1alpha1.LogstashService{{
						Name: LogstashAPIServiceName,
						Service: commonv1.ServiceTemplate{
							Spec: corev1.ServiceSpec{
								Ports: []corev1.ServicePort{
									{Name: LogstashAPIServiceName, Protocol: "TCP", Port: 9200},
								},
							},
						},
					}},
				},
			},
			wantSvc: []corev1.Service{{
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
					ClusterIP: "",
					Ports: []corev1.ServicePort{
						{Name: "api", Protocol: "TCP", Port: 9200},
					},
				},
			}},
		},
		{
			name: "Default service plus one",
			logstash: logstashv1alpha1.Logstash{
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
					}},
				},
			},
			wantSvc: []corev1.Service{
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
			client := k8s.NewFakeClient()
			params := Params{
				Context:  context.Background(),
				Client:   client,
				Logstash: tc.logstash,
			}
			haveSvc, haveAPISvc, err := reconcileServices(params)
			require.NoError(t, err)
			require.Equal(t, len(tc.wantSvc), len(haveSvc))

			for i := range tc.wantSvc {
				comparison.AssertEqual(t, &tc.wantSvc[i], &haveSvc[i])

				if tc.wantSvc[i].Name == "logstash-ls-api" {
					comparison.AssertEqual(t, &tc.wantSvc[i], &haveAPISvc)
				}
			}
		})
	}
}
