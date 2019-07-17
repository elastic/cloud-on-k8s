// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
	for _, sset := range l {
		objs = append(objs, sset.ObjectMeta)
	}
	return objs
}

// RevisionUpdateScheduled returns true if at least one revision update is scheduled.
func (l StatefulSetList) RevisionUpdateScheduled() bool {
	for _, s := range l {
		if s.Status.UpdateRevision != "" && s.Status.UpdateRevision != s.Status.CurrentRevision {
			return true
		}
	}
	return false
}

// PodNames returns the names of the pods for all StatefulSets in the list.
func (l StatefulSetList) PodNames() []string {
	names := make([]string, 0, len(l))
	for _, s := range l {
		names = append(names, PodNames(s)...)
	}
	return names
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
