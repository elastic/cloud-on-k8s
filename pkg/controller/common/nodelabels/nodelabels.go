// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodelabels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nodelabels/initcontainer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	commonvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

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

// MaybeWriteNodeLabelsHashInput writes the sorted, deduplicated list of downward node labels
// for t into writer as hash input. It is a no-op when t has no downward node labels configured.
// Callers use this to roll Pods when the set of requested node labels changes.
func MaybeWriteNodeLabelsHashInput(writer io.Writer, t AnnotationTarget) {
	expectedLabels := t.DownwardNodeLabels()
	if len(expectedLabels) == 0 {
		return
	}
	_, _ = writer.Write([]byte(strings.Join(expectedLabels, ",")))
}

func MaybeAddWaitForAnnotationsInitContainer(builder *defaults.PodTemplateBuilder, t AnnotationTarget, operatorImage string) (*defaults.PodTemplateBuilder, error) {
	expectedLabels := t.DownwardNodeLabels()
	if len(expectedLabels) == 0 {
		return builder, nil
	}

	if operatorImage == "" {
		return nil, fmt.Errorf("failed to add wait for annotations init container at %T: operator image not specified", t)
	}

	downwardAPIVolume := DownwardAPIVolume()
	waitInit, err := WaitForAnnotationsInitContainer(operatorImage, expectedLabels)
	if err != nil {
		return builder, err
	}

	// Record the hash identity before merging the ECK-built init container so that
	// NormalizeTemplateForHash can later distinguish ECK-managed images (stable across
	// operator upgrades) from user-supplied overrides (must participate in the hash).
	// This annotation is ECK-owned and written after user metadata merging so that a
	// user value cannot interfere with update detection.
	identity := initContainerHashIdentity(builder.PodTemplate)
	if builder.PodTemplate.Annotations == nil {
		builder.PodTemplate.Annotations = map[string]string{}
	}
	builder.PodTemplate.Annotations[initcontainer.HashAnnotation] = identity

	builder = builder.
		WithVolumes(downwardAPIVolume.Volume()).
		WithVolumeMounts(downwardAPIVolume.VolumeMount()).
		WithInitContainers(waitInit)
	return builder, nil
}

// initContainerHashIdentity returns the identity string to record in initcontainer.HashAnnotation.
// If the pod template already contains a container named initcontainer.ContainerName with a non-empty
// image, the user explicitly supplied it. Its image participates in the workload hash via the
// pod spec directly (NormalizeTemplateForHash leaves it untouched), so the annotation only needs
// to mark it as user-supplied ("user"). Otherwise ECK provides the image and operator
// patch-upgrades must not roll pods ("managed:<version>").
func initContainerHashIdentity(template corev1.PodTemplateSpec) string {
	for _, c := range template.Spec.InitContainers {
		if c.Name == initcontainer.ContainerName && c.Image != "" {
			return "user"
		}
	}
	return "managed:" + initcontainer.HashVersion
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
	if err := c.Patch(ctx, &pod, client.RawPatch(types.StrategicMergePatchType, mergePatch)); err != nil && !apierrors.IsNotFound(err) {
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

// DownwardAPIVolume returns the downward API volume that exposes the Pod annotations file
// under the path polled by the wait-for-annotations init container.
func DownwardAPIVolume() commonvolume.DownwardAPI {
	return commonvolume.DownwardAPI{}.WithAnnotations(true)
}

// WaitForAnnotationsInitContainer builds an init container that blocks until the operator
// has patched all expectedAnnotations onto the Pod's metadata.annotations. It runs the
// operator binary's "wait-for-annotations" subcommand using the operator's own image,
// which removes any dependency on the stack/component image having a shell or grep.
//
// operatorImage must be non-empty; an error is returned otherwise so callers fail loudly
// rather than silently falling back to the stack image via PodTemplateBuilder.WithInitContainerDefaults.
// Callers must also add the volume returned by DownwardAPIVolume to the Pod.
func WaitForAnnotationsInitContainer(operatorImage string, expectedAnnotations []string) (corev1.Container, error) {
	if operatorImage == "" {
		return corev1.Container{}, errors.New("operator image is required to build the wait-for-annotations init container; " +
			"set the OPERATOR_IMAGE env var or --operator-image flag")
	}

	cmd := []string{
		"/elastic-operator",
		"wait-for-annotations",
		"--file=" + DownwardAPIVolume().AnnotationsFilePath(),
	}
	for _, a := range expectedAnnotations {
		cmd = append(cmd, "--annotation="+a)
	}

	return corev1.Container{
		Name:    initcontainer.ContainerName,
		Image:   operatorImage,
		Command: cmd,
		VolumeMounts: []corev1.VolumeMount{
			DownwardAPIVolume().VolumeMount(),
		},
	}, nil
}
