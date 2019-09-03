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

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

var log = logf.Log.WithName("statefulset")

type StatefulSetList []appsv1.StatefulSet

// RetrieveActualStatefulSets returns the list of existing StatefulSets labeled for the given es cluster.
func RetrieveActualStatefulSets(c k8s.Client, es types.NamespacedName) (StatefulSetList, error) {
	var ssets appsv1.StatefulSetList
	ns := client.InNamespace(es.Namespace)
	matchLabels := label.NewLabelSelectorForElasticsearchClusterName(es.Name)
	err := c.List(&ssets, ns, matchLabels)
	return StatefulSetList(ssets.Items), err
}

// GetByName returns the StatefulSet with the given name, and a bool indicating if the StatefulSet was found.
func (l StatefulSetList) GetByName(ssetName string) (appsv1.StatefulSet, bool) {
	for _, sset := range l {
		if sset.Name == ssetName {
			return sset, true
		}
	}
	return appsv1.StatefulSet{}, false
}

// ObjectMetas returns a list of MetaObject from the StatefulSetList.
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
		pods, err := GetActualPodsForStatefulSet(c, statefulSet)
		if err != nil {
			return nil, err
		}
		allPods = append(allPods, pods...)
	}
	return allPods, nil
}

// PodReconciliationDone returns true if actual existing pods match what is specified in the StatefulSetList.
// It may return false if there are pods in the process of being created (but not created yet)
// or terminated (but not removed yet).
func (l StatefulSetList) PodReconciliationDone(c k8s.Client, es v1alpha1.Elasticsearch) (bool, error) {
	// pods we expect to be there based on StatefulSets spec
	expectedPods := l.PodNames()
	// pods that are there for this cluster
	actualRawPods, err := GetActualPodsForCluster(c, es)
	if err != nil {
		return false, err
	}
	actualPods := k8s.PodNames(actualRawPods)

	// check if they match
	match := len(expectedPods) == len(actualPods) && stringsutil.StringsInSlice(expectedPods, actualPods)
	if !match {
		log.V(1).Info(
			"Pod reconciliation is not done yet",
			"namespace", es.Namespace, "es_name", es.Name,
			"expected_pods", expectedPods, "actual_pods", actualPods,
		)
	}

	return match, nil
}

// DeepCopy returns a copy of the StatefulSetList with no reference to the original StatefulSetList.
func (l StatefulSetList) DeepCopy() StatefulSetList {
	result := make(StatefulSetList, 0, len(l))
	for _, s := range l {
		result = append(result, *s.DeepCopy())
	}
	return result
}

// ESVersionMatch returns true if the ES version for this StatefulSet matches the given condition.
func ESVersionMatch(statefulSet appsv1.StatefulSet, condition func(v version.Version) bool) bool {
	v, err := GetESVersion(statefulSet)
	if err != nil || v == nil {
		log.Error(err, "cannot parse version from StatefulSet", "namespace", statefulSet.Namespace, "name", statefulSet.Name)
		return false
	}
	return condition(*v)
}

// AtLeastOneESVersionMatch returns true if at least one StatefulSet's ES version matches the given condition.
func AtLeastOneESVersionMatch(statefulSets StatefulSetList, condition func(v version.Version) bool) bool {
	for _, s := range statefulSets {
		if ESVersionMatch(s, condition) {
			return true
		}
	}
	return false
}
