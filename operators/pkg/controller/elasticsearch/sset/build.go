// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func HeadlessService(ssetName string) corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ssetName,
			Labels: StatefulSetSelector(ssetName),
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: corev1.ClusterIPNone,
			Selector:  StatefulSetSelector(ssetName),
		},
	}
}

func StatefulSetSelector(ssetName string) map[string]string {
	return map[string]string{
		label.StatefulSetNameLabelName: ssetName,
	}
}

func BuildStatefulSet(es types.NamespacedName, nodes v1alpha1.NodeSpec, podTemplateBuilder version.PodTemplateSpecBuilder) (appsv1.StatefulSet, error) {
	podTemplate, err := podTemplateBuilder(nodes)
	if err != nil {
		return appsv1.StatefulSet{}, err
	}
	statefulSetName := name.StatefulSet(es.Name, nodes.Name)

	// make sure all labels from the sset selector are applied to the pod template
	selector := StatefulSetSelector(statefulSetName)
	for k, v := range selector {
		podTemplate.Labels[k] = v
	}

	labels := StatefulSetSelector(statefulSetName)
	labels[label.ClusterNameLabelName] = es.Name

	sset := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      statefulSetName,
			Labels:    labels,
			// TODO: inherit labels and annotations from the CRD
		},
		Spec: appsv1.StatefulSetSpec{
			// we manage the partition ordinal to orchestrate nodes upgrades
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: &nodes.NodeCount,
				},
			},
			// we don't care much about pods creation ordering, and manage deletion ordering ourselves,
			// so we're fine with the StatefulSet controller spawning all pods in parallel
			PodManagementPolicy: appsv1.ParallelPodManagement,
			// use default revision history limit
			RevisionHistoryLimit: nil,
			// build a headless service per statefulset, matching the statefulset name label
			ServiceName: HeadlessService(statefulSetName).Name,
			Selector: &metav1.LabelSelector{
				MatchLabels: StatefulSetSelector(statefulSetName),
			},

			Replicas:             &nodes.NodeCount,
			VolumeClaimTemplates: nodes.VolumeClaimTemplates,
			Template:             podTemplate,
		},
	}

	// store a hash of the sset resource in its labels for comparison purposes
	sset.Labels = hash.SetTemplateHashLabel(sset.Labels, sset.Spec)

	return sset, nil
}
