// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// PodName returns the name of the pod with the given ordinal for this StatefulSet.
func PodName(ssetName string, ordinal int32) string {
	return fmt.Sprintf("%s-%d", ssetName, ordinal)
}

// PodNames returns the names of the pods for this StatefulSet, according to the number of replicas.
func PodNames(sset appsv1.StatefulSet) []string {
	names := make([]string, 0, GetReplicas(sset))
	for i := int32(0); i < GetReplicas(sset); i++ {
		names = append(names, PodName(sset.Name, i))
	}
	return names
}

// PodRevision returns the StatefulSet revision from this pod labels.
func PodRevision(pod corev1.Pod) string {
	return pod.Labels[appsv1.StatefulSetRevisionLabel]
}

// GetActualPodsForStatefulSet returns the existing pods associated to this StatefulSet.
// The returned pods may not match the expected StatefulSet replicas in a transient situation.
func GetActualPodsForStatefulSet(c k8s.Client, sset appsv1.StatefulSet) ([]corev1.Pod, error) {
	var pods corev1.PodList
	if err := c.List(&client.ListOptions{
		Namespace: sset.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			label.StatefulSetNameLabelName: sset.Name,
		}),
	}, &pods); err != nil {
		return nil, err
	}
	return pods.Items, nil
}

// GetActualPodsForCluster return the existing pods associated to this cluster.
func GetActualPodsForCluster(c k8s.Client, es v1alpha1.Elasticsearch) ([]corev1.Pod, error) {
	var pods corev1.PodList
	if err := c.List(&client.ListOptions{
		Namespace: es.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			label.ClusterNameLabelName: es.Name,
		}),
	}, &pods); err != nil {
		return nil, err
	}
	return pods.Items, nil
}

// GetActualMastersForCluster returns the list of existing master-eligible pods for the cluster.
func GetActualMastersForCluster(c k8s.Client, es v1alpha1.Elasticsearch) ([]corev1.Pod, error) {
	pods, err := GetActualPodsForCluster(c, es)
	if err != nil {
		return nil, err
	}
	return label.FilterMasterNodePods(pods), nil
}

// ScheduledUpgradesDone returns true if all pods scheduled for upgrade have been upgraded.
// This is done by checking the revision of pods whose ordinal is higher or equal than the StatefulSet
// rollingUpdate.Partition index.
func ScheduledUpgradesDone(c k8s.Client, statefulSets StatefulSetList) (bool, error) {
	for _, s := range statefulSets {
		if s.Status.UpdateRevision == "" {
			// no upgrade scheduled
			continue
		}
		partition := GetPartition(s)
		for i := GetReplicas(s) - 1; i >= partition; i-- {
			var pod corev1.Pod
			err := c.Get(types.NamespacedName{Namespace: s.Namespace, Name: PodName(s.Name, i)}, &pod)
			if errors.IsNotFound(err) {
				// pod probably being terminated
				return false, nil
			}
			if err != nil {
				return false, err
			}
			if PodRevision(pod) != s.Status.UpdateRevision {
				return false, nil
			}
		}
	}
	return true, nil
}
