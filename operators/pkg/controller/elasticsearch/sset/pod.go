// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

func PodName(ssetName string, ordinal int32) string {
	return fmt.Sprintf("%s-%d", ssetName, ordinal)
}

func PodNames(sset appsv1.StatefulSet) []string {
	names := make([]string, 0, Replicas(sset))
	for i := int32(0); i < Replicas(sset); i++ {
		names = append(names, PodName(sset.Name, i))
	}
	return names
}

func PodRevision(pod corev1.Pod) string {
	return pod.Labels[appsv1.StatefulSetRevisionLabel]
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
		partition := GetUpdatePartition(s)
		for i := Replicas(s) - 1; i >= partition; i-- {
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
