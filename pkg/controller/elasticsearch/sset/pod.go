// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package sset

import (
	"context"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// GetActualPodsForStatefulSet returns the existing pods associated to this StatefulSet.
// The returned pods may not match the expected StatefulSet replicas in a transient situation.
func GetActualPodsForStatefulSet(c k8s.Client, sset types.NamespacedName) ([]corev1.Pod, error) {
	return statefulset.GetActualPodsForStatefulSet(c, sset, label.StatefulSetNameLabelName)
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

// StatefulSetName returns the name of the statefulset a Pod belongs to.
func StatefulSetName(podName string) (ssetName string, ordinal int32, err error) {
	ordinalPos := strings.LastIndex(podName, "-")
	ordinalAsString := podName[ordinalPos+1:]
	ordinalAsInt, err := strconv.ParseInt(ordinalAsString, 10, 32)
	return podName[:ordinalPos], int32(ordinalAsInt), err
}

// GetESVersion returns the ES version from the StatefulSet labels.
func GetESVersion(statefulSet appsv1.StatefulSet) (version.Version, error) {
	return label.ExtractVersion(statefulSet.Spec.Template.Labels)
}
