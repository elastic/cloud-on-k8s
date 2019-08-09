// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

var log = logf.Log.WithName("statefulset")

type StatefulSetList []appsv1.StatefulSet

// RetrieveActualStatefulSets returns the list of existing StatefulSets labeled for the given es cluster.
func RetrieveActualStatefulSets(c k8s.Client, es types.NamespacedName) (StatefulSetList, error) {
	var ssets appsv1.StatefulSetList
	err := c.List(&client.ListOptions{
		Namespace:     es.Namespace,
		LabelSelector: label.NewLabelSelectorForElasticsearchClusterName(es.Name),
	}, &ssets)
	return StatefulSetList(ssets.Items), err
}

func (l StatefulSetList) GetByName(ssetName string) (appsv1.StatefulSet, bool) {
	for _, sset := range l {
		if sset.Name == ssetName {
			return sset, true
		}
	}
	return appsv1.StatefulSet{}, false
}

func (l StatefulSetList) ObjectMetas() []metav1.ObjectMeta {
	objs := make([]metav1.ObjectMeta, len(l))
	for i, sset := range l {
		objs[i] = sset.ObjectMeta
	}
	return objs
}

// ToUpdate filters the StatefulSetList to the ones having an update revision scheduled.
func (l StatefulSetList) ToUpdate() StatefulSetList {
	toUpdate := StatefulSetList{}
	for _, s := range l {
		if s.Status.UpdateRevision != "" && (s.Status.UpdateRevision != s.Status.CurrentRevision) {
			toUpdate = append(toUpdate, s)
		}
	}
	return toUpdate
}

// PodNames returns the names of the pods for all StatefulSets in the list.
func (l StatefulSetList) PodNames() []string {
	names := make([]string, 0, len(l))
	for _, s := range l {
		names = append(names, PodNames(s)...)
	}
	return names
}

// GetActualPods returns the list of pods currently existing in the StatefulSetList.
func (l StatefulSetList) GetActualPods(c k8s.Client) ([]corev1.Pod, error) {
	allPods := []corev1.Pod{}
	for _, statefulSet := range l {
		pods, err := GetActualPods(c, statefulSet)
		if err != nil {
			return nil, err
		}
		allPods = append(allPods, pods...)
	}
	return allPods, nil
}

// DeepCopy returns a copy of the StatefulSetList with no reference to the original StatefulSetList.
func (l StatefulSetList) DeepCopy() StatefulSetList {
	result := make(StatefulSetList, 0, len(l))
	for _, s := range l {
		result = append(result, *s.DeepCopy())
	}
	return result
}

// GetUpdatePartition returns the updateStrategy.Partition index, or falls back to the number of replicas if not set.
func GetUpdatePartition(statefulSet appsv1.StatefulSet) int32 {
	if statefulSet.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		return *statefulSet.Spec.UpdateStrategy.RollingUpdate.Partition
	}
	if statefulSet.Spec.Replicas != nil {
		return *statefulSet.Spec.Replicas
	}
	return 0
}

func ForStatefulSet(statefulSet appsv1.StatefulSet) (*version.Version, error) {
	return label.ExtractVersion(statefulSet.Spec.Template.Labels)
}

func ESVersionMatch(statefulSet appsv1.StatefulSet, condition func(v version.Version) bool) bool {
	v, err := ForStatefulSet(statefulSet)
	if err != nil || v == nil {
		log.Error(err, "cannot parse version from StatefulSet", "namespace", statefulSet.Namespace, "name", statefulSet.Name)
		return false
	}
	return condition(*v)
}

func AtLeastOneESVersionMatch(statefulSets StatefulSetList, condition func(v version.Version) bool) bool {
	for _, s := range statefulSets {
		if ESVersionMatch(s, condition) {
			return true
		}
	}
	return false
}
