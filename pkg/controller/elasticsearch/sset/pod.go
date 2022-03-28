// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package sset

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
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
func GetActualPodsForStatefulSet(c k8s.Client, sset types.NamespacedName) ([]corev1.Pod, error) {
	var pods corev1.PodList
	ns := client.InNamespace(sset.Namespace)
	matchLabels := client.MatchingLabels(map[string]string{
		label.StatefulSetNameLabelName: sset.Name,
	})
	if err := c.List(context.Background(), &pods, matchLabels, ns); err != nil {
		return nil, err
	}
	return pods.Items, nil
}

// GetActualPodsForCluster return the existing pods associated to this cluster.
func GetActualPodsForCluster(c k8s.Client, es esv1.Elasticsearch) ([]corev1.Pod, error) {
	var pods corev1.PodList

	ns := client.InNamespace(es.Namespace)
	matchLabels := client.MatchingLabels(map[string]string{
		label.ClusterNameLabelName: es.Name,
	})
	if err := c.List(context.Background(), &pods, ns, matchLabels); err != nil {
		return nil, err
	}
	return pods.Items, nil
}

// GetActualMastersForCluster returns the list of existing master-eligible pods for the cluster.
func GetActualMastersForCluster(c k8s.Client, es esv1.Elasticsearch) ([]corev1.Pod, error) {
	var pods corev1.PodList

	ns := client.InNamespace(es.Namespace)
	matchLabels := client.MatchingLabels(map[string]string{
		label.ClusterNameLabelName:             es.Name,
		string(label.NodeTypesMasterLabelName): "true",
	})
	if err := c.List(context.Background(), &pods, ns, matchLabels); err != nil {
		return nil, err
	}
	return pods.Items, nil
}

func pendingPodsForStatefulSet(c k8s.Client, statefulSet appsv1.StatefulSet) ([]string, []string, error) {
	// check all expected pods are there: no more, no less
	actualPods, err := GetActualPodsForStatefulSet(c, k8s.ExtractNamespacedName(&statefulSet))
	if err != nil {
		return nil, nil, err
	}
	actualPodNames := k8s.PodNames(actualPods)
	expectedPodNames := PodNames(statefulSet)
	pendingCreations, pendingDeletions := stringsutil.Difference(expectedPodNames, actualPodNames)
	return pendingCreations, pendingDeletions, nil
}

// StatefulSetName returns the name of the statefulset a Pod belongs to.
func StatefulSetName(podName string) (ssetName string, ordinal int32, err error) {
	ordinalPos := strings.LastIndex(podName, "-")
	ordinalAsString := podName[ordinalPos+1:]
	ordinalAsInt, err := strconv.ParseInt(ordinalAsString, 10, 32)
	return podName[:ordinalPos], int32(ordinalAsInt), err
}
