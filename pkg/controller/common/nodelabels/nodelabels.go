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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// AnnotationTarget is implemented by ECK custom resources whose managed Pods should have
// Kubernetes node labels copied to their annotations via AnnotatePods. Any ECK CR that already
// implements metav1.Object and GetIdentityLabels satisfies this interface once it exposes a
// DownwardNodeLabels accessor.
type AnnotationTarget interface {
	metav1.Object
	// DownwardNodeLabels returns the node labels expected to be copied as annotations on the
	// Pods managed by the resource. An empty result disables node-label propagation.
	DownwardNodeLabels() []string
	// GetIdentityLabels returns the label set identifying Pods managed by the resource.
	GetIdentityLabels() map[string]string
}

// AnnotatePods copies the expected node labels as annotations on all Pods managed by the given
// target. Missing node labels are reported as errors but do not stop the reconciliation of
// other Pods. The call is a no-op when the target has no downward node labels configured.
func AnnotatePods(ctx context.Context, c k8s.Client, t AnnotationTarget) *reconciler.Results {
	span, ctx := apm.StartSpan(ctx, "annotate_pods_with_node_labels", tracing.SpanTypeApp)
	defer span.End()
	results := reconciler.NewResult(ctx)
	expectedLabels := t.DownwardNodeLabels()
	if len(expectedLabels) == 0 {
		return results
	}
	pods, err := k8s.PodsMatchingLabels(c, t.GetNamespace(), t.GetIdentityLabels())
	if err != nil {
		return results.WithError(err)
	}
	for _, pod := range pods {
		results.WithError(annotatePod(ctx, c, pod, expectedLabels, t.GetName()))
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
