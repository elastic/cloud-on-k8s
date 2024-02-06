// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package statefulset

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/stringsutil"
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

func GetActualPodsForStatefulSet(c k8s.Client, sset types.NamespacedName, labelName string) ([]corev1.Pod, error) {
	var pods corev1.PodList
	ns := client.InNamespace(sset.Namespace)
	matchLabels := client.MatchingLabels(map[string]string{
		labelName: sset.Name,
	})
	if err := c.List(context.Background(), &pods, matchLabels, ns); err != nil {
		return nil, err
	}
	return pods.Items, nil
}

// PodReconciliationDone returns true if actual existing pods match what is specified in the StatefulSetList.
// It may return false if there are pods in the process of being:
// - created (but not there in our resources cache)
// - removed (but still there in our resources cache)
// Status of the pods (running, error, etc.) is ignored.
func PodReconciliationDone(ctx context.Context, c k8s.Client, statefulSet appsv1.StatefulSet, labelName string) (bool, string, error) {
	pendingCreations, pendingDeletions, err := PendingPodsForStatefulSet(c, statefulSet, labelName)
	if err != nil {
		return false, "", err
	}
	if len(pendingCreations) > 0 || len(pendingDeletions) > 0 {
		ulog.FromContext(ctx).V(1).Info(
			"Some pods still need to be created/deleted",
			"namespace", statefulSet.Namespace, "statefulset_name", statefulSet.Name,
			"pending_creations", pendingCreations, "pending_deletions", pendingDeletions,
		)

		var reason strings.Builder
		if len(pendingCreations) > 0 {
			reason.WriteString(fmt.Sprintf(", creations: %s", pendingCreations))
		}
		if len(pendingDeletions) > 0 {
			reason.WriteString(fmt.Sprintf(", deletions: %s", pendingDeletions))
		}

		return false, reason.String(), nil
	}
	return true, "", nil
}

func PendingPodsForStatefulSet(c k8s.Client, statefulSet appsv1.StatefulSet, labelName string) ([]string, []string, error) {
	// check all expected pods are there: no more, no less
	actualPods, err := GetActualPodsForStatefulSet(c, k8s.ExtractNamespacedName(&statefulSet), labelName)
	if err != nil {
		return nil, nil, err
	}
	actualPodNames := k8s.PodNames(actualPods)
	expectedPodNames := PodNames(statefulSet)
	pendingCreations, pendingDeletions := stringsutil.Difference(expectedPodNames, actualPodNames)
	return pendingCreations, pendingDeletions, nil
}
