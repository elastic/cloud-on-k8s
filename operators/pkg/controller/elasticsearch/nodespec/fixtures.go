// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodespec

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TestSset struct {
	Name        string
	ClusterName string
	Version     string
	Replicas    int32
	Master      bool
	Data        bool
	Partition   int32
	Status      appsv1.StatefulSetStatus
}

func (t TestSset) Build() appsv1.StatefulSet {
	labels := map[string]string{
		label.VersionLabelName:     t.Version,
		label.ClusterNameLabelName: t.ClusterName,
	}
	label.NodeTypesMasterLabelName.Set(t.Master, labels)
	label.NodeTypesDataLabelName.Set(t.Data, labels)
	return appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: t.Name,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &t.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: "RollingUpdate",
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: &t.Partition,
				},
			},
		},
		Status: t.Status,
	}
}

type TestPod struct {
	Namespace       string
	Name            string
	ClusterName     string
	StatefulSetName string
	Version         string
	Revision        string
	Master          bool
	Data            bool
}

func (t TestPod) Build() corev1.Pod {
	labels := map[string]string{
		label.VersionLabelName:          t.Version,
		label.ClusterNameLabelName:      t.ClusterName,
		label.StatefulSetNameLabelName:  t.StatefulSetName,
		appsv1.StatefulSetRevisionLabel: t.Revision,
	}
	label.NodeTypesMasterLabelName.Set(t.Master, labels)
	label.NodeTypesDataLabelName.Set(t.Data, labels)
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.Namespace,
			Name:      t.Name,
			Labels:    labels,
		},
	}
}

func (t TestPod) BuildPtr() *corev1.Pod {
	pod := t.Build()
	return &pod
}
