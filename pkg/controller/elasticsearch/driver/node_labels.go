// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

// isPodScheduled returns if the Pod is scheduled. If true it also returns the node name.
func isPodScheduled(pod *corev1.Pod) (bool, string) {
	status := pod.Status
	for i := range status.Conditions {
		if !(status.Conditions[i].Type == corev1.PodScheduled) {
			continue
		}
		return status.Conditions[i].Status == corev1.ConditionTrue && pod.Spec.NodeName != "", pod.Spec.NodeName
	}
	return false, ""
}

// annotatePodsWithNodeLabels annotates all the Pods with the expected node labels.
func annotatePodsWithNodeLabels(ctx context.Context, c k8s.Client, es esv1.Elasticsearch) *reconciler.Results {
	span, ctx := apm.StartSpan(ctx, "annotate_pods_with_node_labels", tracing.SpanTypeApp)
	defer span.End()
	results := reconciler.NewResult(ctx)
	if !es.HasDownwardNodeLabels() {
		return results
	}
	actualPods, err := sset.GetActualPodsForCluster(c, es)
	if err != nil {
		return results.WithError(err)
	}
	for _, pod := range actualPods {
		results.WithError(annotatePodWithNodeLabels(ctx, c, pod, es))
	}
	return results
}

func annotatePodWithNodeLabels(ctx context.Context, c k8s.Client, pod corev1.Pod, es esv1.Elasticsearch) error {
	scheduled, nodeName := isPodScheduled(&pod)
	if !scheduled {
		return nil
	}
	node := &corev1.Node{}
	if err := c.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return err
	}
	// Get the missing annotations.
	podAnnotations, err := getPodAnnotations(&pod, es.DownwardNodeLabels(), node.Labels)
	if err != nil {
		return err
	}
	// Stop early if there is no annotation to set.
	if len(podAnnotations) == 0 {
		return nil
	}
	ulog.FromContext(ctx).Info("Setting Pod annotations from node labels", "err", err, "namespace", es.Namespace, "es_name", es.Name, "pod", pod.Name, "annotations", podAnnotations)
	mergePatch, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": podAnnotations,
		},
	})
	if err != nil {
		return err
	}
	if err := c.Patch(ctx, &pod, client.RawPatch(types.StrategicMergePatchType, mergePatch)); err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}

// getPodAnnotations returns missing annotations, and their values, expected on a given Pod.
// It also ensures that labels exist on the K8S node, if not the case an error is returned.
func getPodAnnotations(pod *corev1.Pod, expectedAnnotations []string, nodeLabels map[string]string) (map[string]string, error) {
	podAnnotations := make(map[string]string)
	var missingLabels []string
	for _, expectedAnnotation := range expectedAnnotations {
		value, ok := nodeLabels[expectedAnnotation]
		if !ok {
			missingLabels = append(missingLabels, expectedAnnotation)
			continue
		}
		// Check if the annotations is already set
		if _, alreadyExists := pod.Annotations[expectedAnnotation]; alreadyExists {
			continue
		}
		podAnnotations[expectedAnnotation] = value
	}
	if len(missingLabels) > 0 {
		return nil,
			fmt.Errorf(
				"following annotations are expected to be set on Pod %s/%s but do not exist as node labels: %s",
				pod.Namespace,
				pod.Name,
				strings.Join(missingLabels, ","))
	}
	return podAnnotations, nil
}
