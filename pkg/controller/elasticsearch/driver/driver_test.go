// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func defaultElasticsearchObject() *v1.Elasticsearch {
	return &v1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testcluster",
			Namespace: "testing",
			Labels: map[string]string{
				label.ClusterNameLabelName: "testcluster",
			},
		},
	}
}

func defaultElasticsearchK8sRuntimeObjects(podStatus corev1.PodStatus) []runtime.Object {
	return []runtime.Object{
		defaultElasticsearchObject(),
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testclusterpod0",
				Namespace: "testing",
				Labels: map[string]string{
					label.ClusterNameLabelName: "testcluster",
				},
			},
			Status: podStatus,
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      services.ExternalServiceName("testcluster"),
				Namespace: "testing",
			},
		}}
}

func Test_defaultDriver_isReachable(t *testing.T) {
	type fields struct {
		DefaultDriverParameters DefaultDriverParameters
	}
	tests := []struct {
		name    string
		fields  fields
		want    bool
		wantErr bool
	}{
		{
			"elasticsearch pods with phase running, containers, and pod ready is true",
			fields{
				DefaultDriverParameters: DefaultDriverParameters{
					Client: k8s.NewFakeClient(defaultElasticsearchK8sRuntimeObjects(corev1.PodStatus{
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
						},
					})...),
					ES: *defaultElasticsearchObject(),
				},
			},
			true,
			false,
		},
		{
			"elasticsearch pods with phase failed, containers, and pod ready is false",
			fields{
				DefaultDriverParameters: DefaultDriverParameters{
					Client: k8s.NewFakeClient(defaultElasticsearchK8sRuntimeObjects(corev1.PodStatus{
						Phase: corev1.PodFailed,
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
						},
					})...),
					ES: *defaultElasticsearchObject(),
				},
			},
			false,
			false,
		},
		{
			"elasticsearch pods with phase running, containers and pod not ready is false",
			fields{
				DefaultDriverParameters: DefaultDriverParameters{
					Client: k8s.NewFakeClient(defaultElasticsearchK8sRuntimeObjects(corev1.PodStatus{
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionFalse,
							},
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionFalse,
							},
						},
					})...),
					ES: *defaultElasticsearchObject(),
				},
			},
			false,
			false,
		},
		{
			"elasticsearch pods with phase running, containers ready but pod not ready is false",
			fields{
				DefaultDriverParameters: DefaultDriverParameters{
					Client: k8s.NewFakeClient(defaultElasticsearchK8sRuntimeObjects(corev1.PodStatus{
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionFalse,
							},
						},
					})...),
					ES: *defaultElasticsearchObject(),
				},
			},
			false,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &defaultDriver{
				DefaultDriverParameters: tt.fields.DefaultDriverParameters,
			}
			got, err := d.isReachable()
			if (err != nil) != tt.wantErr {
				t.Errorf("defaultDriver.isReachable() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("defaultDriver.isReachable() = %v, want %v", got, tt.want)
			}
		})
	}
}
