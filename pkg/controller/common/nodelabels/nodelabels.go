// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package nodelabels contains shared helpers to propagate Kubernetes node labels to the annotations
// of Pods managed by ECK. Labels are opt-in via the DownwardNodeLabelsAnnotation on the owning
// custom resource and are copied to the Pod annotations of the resource they are set on.
package nodelabels

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

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/nodelabels"
)

// DownwardNodeLabelsAnnotation is re-exported for convenience.
const DownwardNodeLabelsAnnotation = nodelabels.DownwardNodeLabelsAnnotation

// AnnotatePods copies the expected node labels as annotations on all Pods in the given namespace
// matching the given labelSelector. Missing node labels are reported as errors but do not stop the
// reconciliation of other Pods.
func AnnotatePods(
	ctx context.Context,
	c k8s.Client,
	namespace string,
	podLabelSelector map[string]string,
	expectedLabels []string,
	resourceName string,
) *reconciler.Results {
	span, ctx := apm.StartSpan(ctx, "annotate_pods_with_node_labels", tracing.SpanTypeApp)
	defer span.End()
	results := reconciler.NewResult(ctx)
	if len(expectedLabels) == 0 {
		return results
	}
	pods, err := k8s.PodsMatchingLabels(c, namespace, podLabelSelector)
	if err != nil {
		return results.WithError(err)
	}
	for _, pod := range pods {
		results.WithError(annotatePod(ctx, c, pod, expectedLabels, resourceName))
	}
	return results
}

func annotatePod(ctx context.Context, c k8s.Client, pod corev1.Pod, expectedLabels []string, resourceName string) error {
	scheduled, nodeName := isPodScheduled(&pod)
	if !scheduled {
		return nil
	}
	node := &corev1.Node{}
	if err := c.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return err
	}
	podAnnotations, err := getPodAnnotations(&pod, expectedLabels, node.Labels)
	if err != nil {
		return err
	}
	if len(podAnnotations) == 0 {
		return nil
	}
	ulog.FromContext(ctx).Info(
		"Setting Pod annotations from node labels",
		"namespace", pod.Namespace,
		"resource_name", resourceName,
		"pod", pod.Name,
		"annotations", podAnnotations,
	)
	mergePatch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
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

// isPodScheduled returns whether the Pod is scheduled and the node it is scheduled on.
func isPodScheduled(pod *corev1.Pod) (bool, string) {
	for _, cond := range pod.Status.Conditions {
		if cond.Type != corev1.PodScheduled {
			continue
		}
		return cond.Status == corev1.ConditionTrue && pod.Spec.NodeName != "", pod.Spec.NodeName
	}
	return false, ""
}

// getPodAnnotations returns missing annotations to add on the given Pod derived from its node labels.
// Labels that are expected but missing from the node result in an error.
func getPodAnnotations(pod *corev1.Pod, expectedAnnotations []string, nodeLabels map[string]string) (map[string]string, error) {
	podAnnotations := make(map[string]string)
	var missingLabels []string
	for _, expectedAnnotation := range expectedAnnotations {
		value, ok := nodeLabels[expectedAnnotation]
		if !ok {
			missingLabels = append(missingLabels, expectedAnnotation)
			continue
		}
		if _, alreadyExists := pod.Annotations[expectedAnnotation]; alreadyExists {
			continue
		}
		podAnnotations[expectedAnnotation] = value
	}
	if len(missingLabels) > 0 {
		return nil, fmt.Errorf(
			"following annotations are expected to be set on Pod %s/%s but do not exist as node labels: %s",
			pod.Namespace,
			pod.Name,
			strings.Join(missingLabels, ","),
		)
	}
	return podAnnotations, nil
}
